// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package k8s

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/jonboulle/clockwork"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubectl/pkg/cmd/apply"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	cmdwait "k8s.io/kubectl/pkg/cmd/wait"
	"k8s.io/kubectl/pkg/scheme"
	"k8s.io/kubectl/pkg/util"
	"k8s.io/kubectl/pkg/util/openapi"
)

const (
	defaultRetries = 3
	defaultTimeout = 30 * time.Second
)

// Kubectl provides utilities based on the kubectl API.
type Kubectl struct {
	defaultNamespace string
	factory          cmdutil.Factory
	openAPISchema    openapi.Resources
	out              io.Writer
	errOut           io.Writer
}

// NewKubectl creates a new instance of Kubectl.
func NewKubectl(confFlags *genericclioptions.ConfigFlags) (*Kubectl, error) {
	matchVersionFlags := cmdutil.NewMatchVersionFlags(confFlags)
	factory := cmdutil.NewFactory(matchVersionFlags)

	openAPISchema, err := factory.OpenAPISchema()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve OpenAPI schema: %w", err)
	}

	logger := zap.L().Named("kubectl")

	return &Kubectl{
		defaultNamespace: findActiveNamespace(confFlags),
		factory:          factory,
		openAPISchema:    openAPISchema,
		out:              &outputCapturer{logFunc: logger.Debug},
		errOut:           &outputCapturer{logFunc: logger.Error},
	}, nil
}

// findActiveNamespace figures out the active namespace by considering the `-n` flag and the active context.
func findActiveNamespace(confFlags *genericclioptions.ConfigFlags) string {
	if confFlags.Namespace != nil && *confFlags.Namespace != "" {
		return *confFlags.Namespace
	}

	clientConf := confFlags.ToRawKubeConfigLoader()

	defaultNS, _, err := clientConf.Namespace()
	if err != nil {
		return "default"
	}

	return defaultNS
}

func (h *Kubectl) Namespace() string {
	return h.defaultNamespace
}

// GetResources gets the named objects.
func (h *Kubectl) GetResources(namespace string, names ...string) *resource.Result {
	return h.factory.NewBuilder().
		Unstructured().
		NamespaceParam(namespace).
		DefaultNamespace().
		ResourceTypeOrNameArgs(true, names...).
		ContinueOnError().
		Latest().
		Flatten().
		Do()
}

func (h *Kubectl) GetPods(namespace string, labelSelector string) ([]corev1.Pod, error) {
	clientset, err := h.factory.KubernetesClientSet()
	if err != nil {
		return nil, err
	}
	podList, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, err
	}
	return podList.Items, nil
}

// LoadResources loads manifests from the given file path.
func (h *Kubectl) LoadResources(filePath string) (*resource.Result, error) {
	validator, err := h.factory.Validator(metav1.FieldValidationStrict)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain validator: %w", err)
	}

	return h.factory.NewBuilder().
		Unstructured().
		Schema(validator).
		ContinueOnError().
		NamespaceParam(h.defaultNamespace).
		DefaultNamespace().
		Path(true, filePath).
		Flatten().
		Do(), nil
}

// CreateOrUpdate the given set of resources.
func (h *Kubectl) CreateOrUpdate(resources resource.Visitor) error {
	return resources.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}

		modified, err := util.GetModifiedConfiguration(info.Object, true, unstructured.UnstructuredJSONScheme)
		if err != nil {
			return err
		}

		helper := resource.NewHelper(info.Client, info.Mapping)

		// if the object does not exist, create it.
		if err := info.Get(); err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
			return h.create(info)
		}

		// otherwise, try to patch the existing object
		patcher := &apply.Patcher{
			Mapping:     info.Mapping,
			Helper:      helper,
			BackOff:     clockwork.NewRealClock(),
			Timeout:     defaultTimeout,
			GracePeriod: -1,
			Overwrite:   true,
			Retries:     defaultRetries,
		}

		patchBytes, patchedObject, err := patcher.Patch(info.Object, modified, info.Source, info.Namespace, info.Name, h.errOut)
		if err != nil {
			return err
		}

		_ = info.Refresh(patchedObject, true)

		if string(patchBytes) == "{}" {
			h.print("unchanged", info.Object)
		}

		h.print("configured", info.Object)
		return nil
	})
}

func (h *Kubectl) create(info *resource.Info) error {
	if err := util.CreateApplyAnnotation(info.Object, unstructured.UnstructuredJSONScheme); err != nil {
		return cmdutil.AddSourceToErr("creating", info.Source, err)
	}

	obj, err := resource.NewHelper(info.Client, info.Mapping).
		Create(info.Namespace, true, info.Object)
	if err != nil {
		return cmdutil.AddSourceToErr("creating", info.Source, err)
	}

	_ = info.Refresh(obj, true)

	h.print("created", info.Object)
	return nil
}

// DynamicClient returns a dynamic client.
func (h *Kubectl) DynamicClient() (dynamic.Interface, error) {
	return h.factory.DynamicClient()
}

// GVR converts a GroupVersionKind into a GroupVersionResource using the K8S RESTMapper.
func (h *Kubectl) GVR(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	mapper, err := h.factory.ToRESTMapper()
	if err != nil {
		return schema.GroupVersionResource{}, err
	}

	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}
	return mapping.Resource, nil
}

// K8SClient returns a K8S client.
func (h *Kubectl) K8SClient() (*kubernetes.Clientset, error) {
	return h.factory.KubernetesClientSet()
}

// ReplaceOrCreate the given set of resources.
func (h *Kubectl) ReplaceOrCreate(resources resource.Visitor) error {
	return resources.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}

		// Create a copy of the object read from the manifest. This is required because info.Get() has the side effect
		// of replacing the content of info.Object with what is *already* deployed, leading to a no-op operation if the
		// object must be replaced later using the Replace() function.
		object := info.Object.DeepCopyObject()
		// if the object does not exist, create it.
		if err := info.Get(); err != nil {
			if !errors.IsNotFound(err) {
				return err
			}

			return h.create(info)
		}

		if err := util.CreateOrUpdateAnnotation(false, info.Object, scheme.DefaultJSONEncoder()); err != nil {
			return cmdutil.AddSourceToErr("replacing", info.Source, err)
		}
		obj, err := resource.NewHelper(info.Client, info.Mapping).
			WithFieldManager("kubectl-replace").
			Replace(info.Namespace, info.Name, true, object)
		if err != nil {
			return cmdutil.AddSourceToErr("replacing", info.Source, err)
		}
		if err := info.Refresh(obj, true); err != nil {
			return err
		}
		return nil
	})
}

// Delete the given set of resources from the cluster.
func (h *Kubectl) Delete(resources *resource.Result, timeout time.Duration) error {
	resources = resources.IgnoreErrors(errors.IsNotFound)

	dynamicClient, err := h.DynamicClient()
	if err != nil {
		return fmt.Errorf("failed to get dynamic client: %w", err)
	}

	uidMap := cmdwait.UIDMap{}
	deletedInfos := []*resource.Info{}

	err = resources.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			return err
		}

		deletedInfos = append(deletedInfos, info)

		options := &metav1.DeleteOptions{}
		policy := metav1.DeletePropagationBackground
		options.PropagationPolicy = &policy

		deleteResp, err := resource.NewHelper(info.Client, info.Mapping).DeleteWithOptions(info.Namespace, info.Name, options)
		if err != nil {
			return cmdutil.AddSourceToErr("deleting", info.Source, err)
		}

		resourceLocation := cmdwait.ResourceLocation{
			GroupResource: info.Mapping.Resource.GroupResource(),
			Namespace:     info.Namespace,
			Name:          info.Name,
		}

		if status, ok := deleteResp.(*metav1.Status); ok && status.Details != nil {
			uidMap[resourceLocation] = status.Details.UID
			return nil
		}

		responseMetadata, err := meta.Accessor(deleteResp)
		if err != nil {
			zap.S().Warnw("Unable to find UID of deleted object", "ns", info.Namespace, "obj", info.Name, "error", err)
			return nil
		}

		uidMap[resourceLocation] = responseMetadata.GetUID()

		h.print("deleted", info.Object)
		return nil
	})

	if err != nil {
		return err
	}

	waitOptions := cmdwait.WaitOptions{
		ResourceFinder: genericclioptions.ResourceFinderForResult(resource.InfoListVisitor(deletedInfos)),
		UIDMap:         uidMap,
		DynamicClient:  dynamicClient,
		Timeout:        timeout,
		Printer:        printers.NewDiscardingPrinter(),
		ConditionFn:    cmdwait.IsDeleted,
		IOStreams:      genericclioptions.IOStreams{Out: h.out, ErrOut: h.errOut},
	}

	err = waitOptions.RunWait()
	if errors.IsForbidden(err) || errors.IsMethodNotSupported(err) {
		zap.S().Warnw("Waiting is not supported", "error", err)
		return nil
	}

	return err
}

func (h *Kubectl) print(operation string, obj runtime.Object) error {
	printFlags := genericclioptions.NewPrintFlags(operation).WithTypeSetter(scheme.Scheme)

	printer, err := printFlags.ToPrinter()
	if err != nil {
		return err
	}

	return printer.PrintObj(obj, h.out)
}

// outputCapturer exposes an io.Writer interface that outputs to the configured logger.
type outputCapturer struct {
	logFunc func(msg string, fields ...zap.Field)
}

func (oc *outputCapturer) Write(p []byte) (int, error) {
	p = bytes.TrimSpace(p)
	oc.logFunc(string(p))

	return len(p), nil
}
