// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package certificates

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

var utf8ValuedOtherName = UTF8StringValuedOtherName{
	OID:   CommonNameObjectIdentifier,
	Value: "hello.world",
}
var helloOtherName OtherName

var dnsName = GeneralName{DNSName: "hello.world"}

func init() {
	on, err := (&utf8ValuedOtherName).ToOtherName()
	if err != nil {
		panic(err)
	}
	helloOtherName = *on
}

func BenchmarkParseSANGeneralNames0(b *testing.B) {
	benchmarkParseSANGeneralNames(b, 0, 0)
}

func BenchmarkParseSANGeneralNames1(b *testing.B) {
	benchmarkParseSANGeneralNames(b, 1, 0)
}

func BenchmarkParseSANGeneralNames2(b *testing.B) {
	benchmarkParseSANGeneralNames(b, 2, 0)
}

func BenchmarkParseSANGeneralNames0WithDNS1(b *testing.B) {
	benchmarkParseSANGeneralNames(b, 0, 1)
}

func BenchmarkParseSANGeneralNames1WithDNS1(b *testing.B) {
	benchmarkParseSANGeneralNames(b, 1, 1)
}

func BenchmarkParseSANGeneralNames2WithDNS2(b *testing.B) {
	benchmarkParseSANGeneralNames(b, 2, 2)
}

func BenchmarkParseSANGeneralNames3WithDNS3(b *testing.B) {
	benchmarkParseSANGeneralNames(b, 3, 3)
}

//nolint:thelper
func benchmarkParseSANGeneralNames(b *testing.B, otherNames, dnsNames int) {
	var generalNames []GeneralName
	for i := 0; i < otherNames; i++ {
		generalNames = append(generalNames, GeneralName{OtherName: helloOtherName})
	}
	for i := 0; i < dnsNames; i++ {
		generalNames = append(generalNames, dnsName)
	}

	generalNamesBytes, err := MarshalToSubjectAlternativeNamesData(generalNames)
	require.NoError(b, err)

	cert := &x509.Certificate{
		Extensions: []pkix.Extension{
			{Id: SubjectAlternativeNamesObjectIdentifier, Value: generalNamesBytes},
		},
	}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_, err := ParseSANGeneralNamesOtherNamesOnly(cert)
		require.NoError(b, err)
	}
}

func BenchmarkOtherName_ToUTF8StringValuedOtherName(b *testing.B) {
	for n := 0; n < b.N; n++ {
		_, err := helloOtherName.ToUTF8StringValuedOtherName()
		require.NoError(b, err)
	}
}

func ExampleMarshalToSubjectAlternativeNamesData() {
	otherName, err := (&UTF8StringValuedOtherName{OID: CommonNameObjectIdentifier, Value: "foo"}).ToOtherName()

	if err != nil {
		panic(err)
	}

	generalNames := []GeneralName{{OtherName: *otherName}}

	data, err := MarshalToSubjectAlternativeNamesData(generalNames)
	if err != nil {
		panic(err)
	}

	ext := pkix.Extension{Id: SubjectAlternativeNamesObjectIdentifier, Critical: false, Value: data}
	fmt.Println(ext)
	// Output: {2.5.29.17 false [48 14 160 12 6 3 85 4 3 160 5 12 3 102 111 111]}
}

func ExampleParseSANGeneralNamesOtherNamesOnly() {
	generalNames := []GeneralName{{OtherName: helloOtherName}}
	generalNamesBytes, err := MarshalToSubjectAlternativeNamesData(generalNames)
	if err != nil {
		panic(err)
	}

	cert := &x509.Certificate{
		Extensions: []pkix.Extension{
			{Id: SubjectAlternativeNamesObjectIdentifier, Value: generalNamesBytes},
		},
	}

	otherNames, err := ParseSANGeneralNamesOtherNamesOnly(cert)
	if err != nil {
		panic(err)
	}

	fmt.Println(reflect.DeepEqual(generalNames, otherNames))
	// Output: true
}
