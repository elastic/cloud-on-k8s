// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package certificates

import (
	"crypto/x509"
	"encoding/asn1"
	"errors"
)

var (

	// SubjectAlternativeNamesObjectIdentifier is the OID for the Subject Alternative Name x509 extension
	SubjectAlternativeNamesObjectIdentifier = asn1.ObjectIdentifier{2, 5, 29, 17}

	// CommonNameObjectIdentifier is the OID for a CommonName field in x509
	CommonNameObjectIdentifier = asn1.ObjectIdentifier{2, 5, 4, 3}
)

/*
GeneralName is a partially modelled GeneralName from RFC 5280, Section 4.2.1.6

The RFC defines the Subject Alternative Names value as follows:

    id-ce-subjectAltName OBJECT IDENTIFIER ::=  { id-ce 17 }

    SubjectAltName ::= GeneralNames

    GeneralNames ::= SEQUENCE SIZE (1..MAX) OF GeneralName

    GeneralName ::= CHOICE {
        otherName                       [0]     OtherName,
        rfc822Name                      [1]     IA5String,
        dNSName                         [2]     IA5String,
        x400Address                     [3]     ORAddress,
        directoryName                   [4]     Name,
        ediPartyName                    [5]     EDIPartyName,
        uniformResourceIdentifier       [6]     IA5String,
        iPAddress                       [7]     OCTET STRING,
        registeredID                    [8]     OBJECT IDENTIFIER }

    OtherName ::= SEQUENCE {
        type-id    OBJECT IDENTIFIER,
        value      [0] EXPLICIT ANY DEFINED BY type-id }

OtherName is used in Elasticsearch certificates as the node names, and is what is compared to the allowed subjects
in the trust_restrictions file (if configured) when doing certificate validation between ES nodes.

We only model OtherName, DNSName and IPAddress here because those are what we use for the Elasticsearch certs
*/
type GeneralName struct {
	OtherName OtherName `asn1:"optional,tag:0"`
	DNSName   string    `asn1:"optional,ia5,tag:2"`
	IPAddress []byte    `asn1:"optional,tag:7"`
}

// OtherName is a record that contains custom data. The OID defines how the Value should be parsed.
type OtherName struct {
	OID   asn1.ObjectIdentifier
	Value asn1.RawValue
}

// ToUTF8StringValuedOtherName converts the OtherName instance into an UTF8StringValuedOtherName
func (n *OtherName) ToUTF8StringValuedOtherName() (*UTF8StringValuedOtherName, error) {
	var utf8StringValuedOtherName UTF8StringValuedOtherName

	if err := convertASN1(*n, &utf8StringValuedOtherName); err != nil {
		return nil, err
	}

	return &utf8StringValuedOtherName, nil
}

// UTF8StringValuedOtherName is a concrete OtherValue where the Value is a utf8 string.
type UTF8StringValuedOtherName struct {
	OID   asn1.ObjectIdentifier
	Value string `asn1:"utf8,explicit"` // like openssl
}

// ToOtherName converts the UTF8StringValuedOtherName instance into an OtherName
func (n *UTF8StringValuedOtherName) ToOtherName() (*OtherName, error) {
	var otherName OtherName

	if err := convertASN1(*n, &otherName); err != nil {
		return nil, err
	}

	return &otherName, nil
}

// convertASN1 converts a struct to another through asn1 marshalling and unmarshalling
func convertASN1(from, to interface{}) error {
	data, err := asn1.Marshal(from)
	if err != nil {
		return err
	}
	if rest, err := asn1.Unmarshal(data, to); err != nil {
		return err
	} else if len(rest) != 0 {
		return asn1.StructuralError{Msg: "trailing data after unmarshalling"}
	}

	return nil
}

// MarshalToSubjectAlternativeNamesData marshals the provided General Names to a valid value for an X509 SAN extension
func MarshalToSubjectAlternativeNamesData(generalNames []GeneralName) ([]byte, error) {
	sanData, err := asn1.Marshal(generalNames)
	if err != nil {
		return nil, err
	}
	// somehow go wraps each entry as its own sequence, so we need to unwrap each entry to produce a valid SAN
	return flattenNestedASN1Sequence(sanData)
}

// flattenNestedASN1Sequence flattens one level of nested sequences in asn1-encoded data:
// e.g: [[1], [2], [3]] -> [1,2,3]
func flattenNestedASN1Sequence(b []byte) ([]byte, error) {
	var value asn1.RawValue
	rest, err := asn1.Unmarshal(b, &value)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, errors.New("trailing asn1 data")
	}

	var unwrappedBytes []byte
	rest = value.Bytes
	for len(rest) > 0 {
		var nestedValue asn1.RawValue
		rest, err = asn1.Unmarshal(rest, &nestedValue)
		if err != nil {
			return nil, err
		}

		unwrappedBytes = append(unwrappedBytes, nestedValue.Bytes...)
	}
	value.Bytes = unwrappedBytes
	value.FullBytes = nil

	b, err = asn1.Marshal(value)
	return b, err
}

// ParseSANGeneralNamesOtherNamesOnly parses the X509 Subject Alternative Names extensions of a X509 certificate and
// returns a list of GeneralName entries.
//
// Note: Only OtherName entries are returned, any other entry is ignored.
func ParseSANGeneralNamesOtherNamesOnly(c *x509.Certificate) ([]GeneralName, error) {
	var generalNames []GeneralName
	for _, ext := range c.Extensions {
		if SubjectAlternativeNamesObjectIdentifier.Equal(ext.Id) {
			// rfc: should be wrapped in a sequence node:
			var generalNamesValue asn1.RawValue
			rest, err := asn1.Unmarshal(ext.Value, &generalNamesValue)
			if err != nil {
				return nil, err
			}
			if len(rest) != 0 {
				return nil, errors.New("trailing data after SubjectAlternativeNames")
			}

			if generalNamesValue.Class != asn1.ClassUniversal || generalNamesValue.Tag != asn1.TagSequence {
				return nil, errors.New("invalid GeneralNames class or tag")
			}

			rest = generalNamesValue.Bytes
			for len(rest) != 0 {
				var generalName asn1.RawValue
				rest, err = asn1.Unmarshal(rest, &generalName)
				if err != nil {
					return nil, err
				}

				if generalName.Class == asn1.ClassContextSpecific {
					switch generalName.Tag {
					case 0:
						// OtherName ::= SEQUENCE {
						//   type-id    OBJECT IDENTIFIER,
						//   value      [0] EXPLICIT ANY DEFINED BY type-id }

						var otherNameTypeObjectIdentifier asn1.ObjectIdentifier

						otherNameValueBytes, err := asn1.Unmarshal(generalName.Bytes, &otherNameTypeObjectIdentifier)
						if err != nil {
							return nil, err
						}

						var value asn1.RawValue
						vrest, err := asn1.Unmarshal(otherNameValueBytes, &value)
						if err != nil {
							return nil, err
						}
						if len(vrest) != 0 {
							return nil, errors.New("trailing data after OtherName value")
						}

						generalNames = append(generalNames, GeneralName{
							OtherName: OtherName{
								OID:   otherNameTypeObjectIdentifier,
								Value: value,
							},
						})
					default:
						log.Info("Ignoring unsupported GeneralNames tag", "tag", generalName.Tag, "subject", c.Subject)
					}
				}
			}
		}
	}

	return generalNames, nil
}
