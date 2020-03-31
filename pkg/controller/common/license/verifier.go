// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package license

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"time"

	errors2 "github.com/pkg/errors"
)

// Verifier verifies Enterprise licenses.
type Verifier struct {
	PublicKey *rsa.PublicKey
}

// Valid checks the validity of the given Enterprise license.
func (v *Verifier) Valid(l EnterpriseLicense, now time.Time) LicenseStatus {
	if !l.IsValid(now) {
		return LicenseStatusExpired
	}
	if err := v.ValidSignature(l); err != nil {
		log.Error(err, "Failed signature check")
		return LicenseStatusInvalid
	}
	return LicenseStatusValid
}

// ValidSignature checks signature of the given Enterprise license. Returns nil if valid.
func (v *Verifier) ValidSignature(l EnterpriseLicense) error {
	allParts := make([]byte, base64.StdEncoding.DecodedLen(len(l.License.Signature)))
	_, err := base64.StdEncoding.Decode(allParts, []byte(l.License.Signature))
	if err != nil {
		return errors2.Wrap(err, "failed to base64 decode signature")
	}
	buf := bytes.NewBuffer(allParts)
	maxLen := uint32(len(allParts))

	var version uint32
	if err := readInt(buf, &version); err != nil {
		return errors2.Wrap(err, "failed to read version")
	}

	var magicLen uint32
	if err := readInt(buf, &magicLen); err != nil {
		return errors2.Wrap(err, "failed to read magic length")
	}
	if magicLen > maxLen {
		return errors.New("magic exceeds max length")
	}
	magic := make([]byte, magicLen)
	_, err = buf.Read(magic)
	if err != nil {
		return errors2.Wrap(err, "failed to read magic")
	}

	var hashLen uint32
	if err := readInt(buf, &hashLen); err != nil {
		return errors2.Wrap(err, "failed to read hash length")
	}
	if hashLen > maxLen {
		return errors.New("hash exceeds max len")
	}
	pubKeyFingerprint := make([]byte, hashLen)
	_, err = buf.Read(pubKeyFingerprint)
	if err != nil {
		return err
	}
	var signedContentLen uint32
	if err := readInt(buf, &signedContentLen); err != nil {
		return errors2.Wrap(err, "failed to read signed content length")
	}
	if signedContentLen > maxLen {
		return errors.New("signed content exceeds max length")
	}
	signedContentSig := make([]byte, signedContentLen)
	_, err = buf.Read(signedContentSig)
	if err != nil {
		return err
	}
	contentBytes, err := json.Marshal(toVerifiableSpec(l))
	if err != nil {
		return err
	}
	// TODO optional pubkey fingerprint check
	hashed := sha512.Sum512(contentBytes)
	return rsa.VerifyPKCS1v15(v.PublicKey, crypto.SHA512, hashed[:], signedContentSig)
}

// NewVerifier creates a new license verifier from a DER encoded public key.
func NewVerifier(pubKeyBytes []byte) (*Verifier, error) {
	key, err := ParsePubKey(pubKeyBytes)
	return &Verifier{
		PublicKey: key,
	}, err
}

func ParsePubKey(pubKeyBytes []byte) (*rsa.PublicKey, error) {
	pub, err := x509.ParsePKIXPublicKey(pubKeyBytes)
	if err != nil {
		return nil, err
	}
	pubKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("public key is not an RSA key")
	}
	return pubKey, nil
}

// Signer signs Enterprise licenses.
type Signer struct {
	Verifier
	privateKey *rsa.PrivateKey
}

// NewSigner creates a new license signer from a private key.
func NewSigner(privKey *rsa.PrivateKey) *Signer {
	return &Signer{
		Verifier: Verifier{
			PublicKey: &privKey.PublicKey,
		},
		privateKey: privKey,
	}
}

// Sign signs the given Enterprise license.
func (s *Signer) Sign(l EnterpriseLicense) ([]byte, error) {
	spec := toVerifiableSpec(l)
	toSign, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	rng := rand.Reader
	hashed := sha512.Sum512(toSign)

	publicKeyBytes := x509.MarshalPKCS1PublicKey(s.PublicKey)
	rsaSig, err := rsa.SignPKCS1v15(rng, s.privateKey, crypto.SHA512, hashed[:])
	if err != nil {
		return nil, err
	}
	const magicLen = 13
	magic := make([]byte, magicLen)
	_, err = rand.Read(magic)
	if err != nil {
		return nil, err
	}
	hash := make([]byte, base64.StdEncoding.EncodedLen(len(publicKeyBytes)))
	base64.StdEncoding.Encode(hash, publicKeyBytes)
	// version (uint32) + magic length (uint32) + magic + hash length (uint32) + hash + sig length (uint32) + sig
	sig := make([]byte, 0, 4+4+magicLen+4+len(hash)+4+len(rsaSig))

	buf := bytes.NewBuffer(sig)

	if err := writeInt(buf, l.License.Version); err != nil {
		return nil, err
	}
	if err := writeInt(buf, len(magic)); err != nil {
		return nil, err
	}
	_, err = buf.Write(magic)
	if err != nil {
		return nil, err
	}
	if err := writeInt(buf, len(hash)); err != nil {
		return nil, err
	}
	_, err = buf.Write(hash)
	if err != nil {
		return nil, err
	}
	if err := writeInt(buf, len(rsaSig)); err != nil {
		return nil, err
	}
	_, err = buf.Write(rsaSig)
	if err != nil {
		return nil, err
	}
	sigBytes := buf.Bytes()
	out := make([]byte, base64.StdEncoding.EncodedLen(len(sigBytes)))
	base64.StdEncoding.Encode(out, sigBytes)
	return out, nil
}

type licenseSpec struct {
	UID                string `json:"uid"`
	LicenseType        string `json:"type"`
	IssueDate          string `json:"issue_date,omitempty"`
	StartDate          string `json:"start_date,omitempty"`
	ExpiryDate         string `json:"expiry_date,omitempty"`
	IssueDateInMillis  int64  `json:"issue_date_in_millis,omitempty"`
	StartDateInMillis  int64  `json:"start_date_in_millis,omitempty"`
	ExpiryDateInMillis int64  `json:"expiry_date_in_millis,omitempty"`
	MaxInstances       int    `json:"max_instances,omitempty"`
	MaxResourceUnits   int    `json:"max_resource_units,omitempty"`
	IssuedTo           string `json:"issued_to"`
	Issuer             string `json:"issuer"`
}

func toVerifiableSpec(l EnterpriseLicense) licenseSpec {
	return licenseSpec{
		UID:                l.License.UID,
		LicenseType:        string(l.License.Type),
		IssueDateInMillis:  l.License.IssueDateInMillis,
		StartDateInMillis:  l.License.StartDateInMillis,
		ExpiryDateInMillis: l.License.ExpiryDateInMillis,
		MaxInstances:       l.License.MaxInstances,
		MaxResourceUnits:   l.License.MaxResourceUnits,
		IssuedTo:           l.License.IssuedTo,
		Issuer:             l.License.Issuer,
	}
}

func writeInt(buffer *bytes.Buffer, i int) error {
	in := make([]byte, 4)
	binary.BigEndian.PutUint32(in, uint32(i))
	_, err := buffer.Write(in)
	return err
}

func readInt(r io.Reader, i *uint32) error {
	out := make([]byte, 4)
	_, err := io.ReadFull(r, out)
	if err != nil {
		return err
	}
	*i = binary.BigEndian.Uint32(out)
	return nil
}
