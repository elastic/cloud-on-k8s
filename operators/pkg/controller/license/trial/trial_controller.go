/*
 * Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
 * or more contributor license agreements. Licensed under the Elastic License;
 * you may not use this file except in compliance with the Elastic License.
 */

package trial

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"sync/atomic"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	licensing "github.com/elastic/k8s-operators/operators/pkg/controller/common/license"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/operator"
	"github.com/elastic/k8s-operators/operators/pkg/controller/license/mutation"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	log = logf.Log.WithName("trial-controller")
)

const (
	trialStatusSecretKey = "trial-status"
	pubkeyKey            = "pubkey"
	signatureKey         = "signature"
	finalizerName        = "trial/finalizers.k8s.elastic.co" // slash required on core object finalizers to be fully qualified
)

// Reconcile watches enterprise trial licenses. If it finds a trial license it checks whether a trial has been started.
// If not it starts the trial period.
// If a trial is already running it validates the trial license and updates its status.
func (r *ReconcileTrials) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// atomically update the iteration to support concurrent runs.
	currentIteration := atomic.AddInt64(&r.iteration, 1)
	iterationStartTime := time.Now()
	log.Info("Start reconcile iteration", "iteration", currentIteration, "request", request)
	defer func() {
		log.Info("End reconcile iteration", "iteration", currentIteration, "took", time.Since(iterationStartTime))
	}()
	// Fetch the license to ensure it still exists
	license := v1alpha1.EnterpriseLicense{}
	err := r.Get(request.NamespacedName, &license)
	if err != nil {
		if errors.IsNotFound(err) {
			// nothing to do no license
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, pkgerrors.Wrap(err, "Failed to get EnterpriseLicense")
	}

	if !license.DeletionTimestamp.IsZero() || !license.IsTrial() {
		// license is not a trial or  being deleted nothing to do
		return reconcile.Result{}, nil
	}

	err = mutation.PopulateTrialLicense(&license)
	if err != nil {
		return reconcile.Result{}, pkgerrors.Wrap(err, "Failed to populate trial license")
	}
	// 1. fetch trial state secret
	var trialStatus corev1.Secret
	err = r.Get(types.NamespacedName{Namespace: license.Namespace, Name: trialStatusSecretKey}, &trialStatus)
	if errors.IsNotFound(err) {
		// 2. if not present create one + finalizer
		err := r.initTrial(license)
		if err != nil {
			return reconcile.Result{}, pkgerrors.Wrap(err, "Failed to init trial")
		}
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, pkgerrors.Wrap(err, "Failed to retrieve trial status")
	}
	// 3. if present check still valid
	verifier, err := licensing.NewVerifier(trialStatus.Data[pubkeyKey])
	if err != nil {
		return reconcile.Result{}, pkgerrors.Wrap(err, "Failed to initialise license verifier")
	}
	err = verifier.Valid(license, trialStatus.Data[signatureKey])
	if err != nil {
		return reconcile.Result{}, r.updateStatus(license, v1alpha1.LicenseStatusInvalid)
	}
	if !license.IsValid(time.Now()) {
		return reconcile.Result{}, r.updateStatus(license, v1alpha1.LicenseStatusExpired)
	}
	return reconcile.Result{}, r.updateStatus(license, v1alpha1.LicenseStatusValid)
}

func (r *ReconcileTrials) initTrial(l v1alpha1.EnterpriseLicense) error {
	mutation.StartTrial(&l)
	log.Info("Starting enterprise trial", "start", l.StartDate(), "end", l.ExpiryDate())
	rnd := rand.Reader
	tmpPrivKey, err := rsa.GenerateKey(rnd, 2048)
	if err != nil {
		return err
	}
	signer := licensing.NewSigner(tmpPrivKey)
	sig, err := signer.Sign(l)
	if err != nil {
		return pkgerrors.Wrap(err, "Failed to sign license")
	}
	pubkeyBytes, err := x509.MarshalPKIXPublicKey(&tmpPrivKey.PublicKey)
	if err != nil {
		return pkgerrors.Wrap(err, "Failed to marshal public key for trial status")
	}
	trialStatus := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: l.Namespace,
			Name:      trialStatusSecretKey,
			Finalizers: []string{
				finalizerName,
			},
		},
		Data: map[string][]byte{
			signatureKey: sig,
			pubkeyKey:    pubkeyBytes,
		},
	}
	err = r.Create(&trialStatus)
	if err != nil {
		return pkgerrors.Wrap(err, "Failed to created trial status")
	}
	l.Finalizers = append(l.Finalizers, finalizerName)
	l.Spec.SignatureRef = corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: trialStatusSecretKey,
		},
		Key: signatureKey,
	}
	l.Status.LicenseStatus = v1alpha1.LicenseStatusValid
	return pkgerrors.Wrap(r.Update(&l), "Failed to update trial license")
}

func (r *ReconcileTrials) updateStatus(l v1alpha1.EnterpriseLicense, status v1alpha1.LicenseStatus) error {
	log.Info("trial status update", "status", status)
	l.Status.LicenseStatus = status
	return r.Status().Update(&l)
}

// Add creates a new EnterpriseLicense Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, _ operator.Parameters) error {
	r := &ReconcileTrials{Client: k8s.WrapClient(mgr.GetClient()), scheme: mgr.GetScheme()}
	// Create a new controller
	c, err := controller.New("trial-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Enterprise licenses.
	if err := c.Watch(
		&source.Kind{Type: &v1alpha1.EnterpriseLicense{}}, &handler.EnqueueRequestForObject{},
	); err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &ReconcileTrials{}

// ReconcileTrials reconciles Enterprise trial licenses.
type ReconcileTrials struct {
	k8s.Client
	scheme *runtime.Scheme
	// iteration is the number of times this controller has run its Reconcile method
	iteration int64
}
