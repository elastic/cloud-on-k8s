// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package http

import (
	"reflect"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/common/certificates"
)

const (
	ca = `
-----BEGIN CERTIFICATE-----
MIIC/zCCAeegAwIBAgIJAIVZ8xw3LMNkMA0GCSqGSIb3DQEBCwUAMBYxFDASBgNV
BAMMC21vcmVsbG8ub3ZoMB4XDTE5MDgwOTA5MzQwMFoXDTI5MDgwNjA5MzQwMFow
FjEUMBIGA1UEAwwLbW9yZWxsby5vdmgwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAw
ggEKAoIBAQCoM2HYyuTTlu41SlgVO0Hdx7eUQevGSKO6pjPjN49/KKY1z/3DoKzr
seWaGOjiWUAqx/GHX8AsR9ToVoKGBbSNeDxT33pt3I9aCnnOPTt3yDIOlr4ZWnKq
NnNHwfydsMBfBAYgdU/L506KuNHJQ18Zey5+A0roTWyHUT48mQBsjetXg77RfDMB
MYVOWETfl70GKAaAlVGZfJHCkfBzYnPcEjqtcuU/7d27WZrSMhXifzHAEmm0KPER
EWdo4UHTK23wLY6dvkp2O5i0bKHv+PuLpqYrm7R7SWGhhwD651n5S5W20FHDow+d
js0yW2gqYsZZN6S1uAsJ8rdYAEPhK9J9AgMBAAGjUDBOMB0GA1UdDgQWBBQ6Lsen
0HbE+7M6iV9r8n5rZrbl4jAfBgNVHSMEGDAWgBQ6Lsen0HbE+7M6iV9r8n5rZrbl
4jAMBgNVHRMEBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQAgrLJnK4s/OVnh8CRk
GmikP+ZxhDs4k1nlr7+rTYkU0huoHK8p802w4zd74szYsHpo8kON/zSmFD7JpU4L
o2kseENqMsgrCPhF3+TDwf/Li43pbK162iAq8ZEpYnSXbQsRyP+Tz0lzoEoli6o7
6KVn4VNookLMyhGIAOmhfbNm0jG+B2zz+bvoTAe9CiDfvq1k0fnuKFzRtRsj09NJ
FNMhSc02N4EDrGpL5CYmEXjPZS3lUsoYPwbYlmUt3Bzuf5hI0mDHCt3BYKH1vFI4
W8/h9wwGn/yytsH21dkj41KEQK6N65gT9i0fBBiubuS2H1SVMMJ/J7PUqol278Ar
zGpS
-----END CERTIFICATE-----
`
	tls = `
-----BEGIN CERTIFICATE-----
MIIDnzCCAoegAwIBAgIRAKtKtQKtGFIUneRz5r1FnUMwDQYJKoZIhvcNAQELBQAw
FjEUMBIGA1UEAwwLbW9yZWxsby5vdmgwHhcNMTkwODA5MDkzOTIyWhcNMTkxMTA3
MDkzOTIyWjBOMRkwFwYDVQQKExBFbGFzdGljc2VhcmNoIENBMTEwLwYDVQQDEyhl
bGFzdGljc2VhcmNoLXNhbXBsZS1lcy1odHRwLmRlZmF1bHQuc3ZjMIIBIjANBgkq
hkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAq6HRcrfV1kHnXv5Z+ImkgKDvxCezI3/p
yiR0jSv6L7+bblHzzsqkPnz3aaIPJJ2G4sdwaIhl5rJdOvCj48It8OtRidZjzuJH
hN2RpN2Ii5WX4D1u18CrjEQrRUzs/vuwpyP0zWx0yP3lp88fy8kfWHj8cE06KZ3c
jq1fTRjEDv/N6xofqBSIHPsnvOVIP0Sp9bJkw5yO0H3oBfrqP0N2mjnwQknclz30
t/LoXHcRrZTOH42pgG5ODZslqLNgKLXQHzRcglzNQPwYKYHigBiy+xsHxbIIXe1n
R70PYKXisA0bhHTiV1Sa77dqQRdSkm0JzrNg58lHZYA1sVKTh0nRMQIDAQABo4Gv
MIGsMA4GA1UdDwEB/wQEAwIFoDAMBgNVHRMBAf8EAjAAMB8GA1UdIwQYMBaAFDou
x6fQdsT7szqJX2vyfmtmtuXiMGsGA1UdEQRkMGKCKGVsYXN0aWNzZWFyY2gtc2Ft
cGxlLWVzLWh0dHAuZGVmYXVsdC5zdmOCNmVsYXN0aWNzZWFyY2gtc2FtcGxlLWVz
LWh0dHAuZGVmYXVsdC5zdmMuY2x1c3Rlci5sb2NhbDANBgkqhkiG9w0BAQsFAAOC
AQEAL0EBOx2vPXJSIjv8t0S2HkbCSerdDvGSNtkOrTizBtL7EwRSec6nes6OaWo6
JYVNCP0Y+a4jQQrD9MkFKniKxluvLgbsHHsCnQC5tI5iwaOIZe+33pVyNksTc3CC
l2s6Imqpvt6S3GyuWhcwWhwi3pK0ce9RqoO7GONHZmyuOaHGm1OxPeXJQYu7gTKg
3hMjnNAzLOF1oOIrPKnkxfP4jdOrQE1oKk9QR7ScIKLVHJTJoogCM50I7yD7HnMT
itkHwZhk5ptdA29P/OAcZheO5NOGlWJ6OeQl35A9SxgB3DSRTFORoEBfwPZB4ZLC
zODbmFEr7N0FzCN6hU8PjcLLhg==
-----END CERTIFICATE-----
`

	key = `
-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAq6HRcrfV1kHnXv5Z+ImkgKDvxCezI3/pyiR0jSv6L7+bblHz
zsqkPnz3aaIPJJ2G4sdwaIhl5rJdOvCj48It8OtRidZjzuJHhN2RpN2Ii5WX4D1u
18CrjEQrRUzs/vuwpyP0zWx0yP3lp88fy8kfWHj8cE06KZ3cjq1fTRjEDv/N6xof
qBSIHPsnvOVIP0Sp9bJkw5yO0H3oBfrqP0N2mjnwQknclz30t/LoXHcRrZTOH42p
gG5ODZslqLNgKLXQHzRcglzNQPwYKYHigBiy+xsHxbIIXe1nR70PYKXisA0bhHTi
V1Sa77dqQRdSkm0JzrNg58lHZYA1sVKTh0nRMQIDAQABAoIBAHZmCPDUdMV7bTsQ
x8w2V68MVprAsEl7AjKad3Szs8Ggsn6mNkSfcjJRTvQmAcBGkzh6UMcr4PAGd14j
h0ulNsAN9Y/av7uGScQUfVZ4JKv2JHFir8ZSeYUnuZny+ULlKfYDTeswOFg3Hmhm
8A5Kzj7gJ3TpMYhoCDC81ROAVC/rkinM3bGm/JFl9MeSBLleWVGVF7S7dyirN0xi
z4pwleF3LwI9N52l0qAg7InFveBWrW2H8UB6PrMW2oLLw04p1IXy0ja+T9qZHUxp
zJhTzhhXAY+5QDpGwvTqKYKkqhusBUTwGx39/p9Eq05wXkIEnh7Q09kGz7RJcUnD
6ji12skCgYEA2MsRy1Jrk9MZ8XO2EknuHEpDPqRcwJG4CvBp+V569Zq7pmuqWRSz
U2XcHvzBy6d2LS4QzuHO/YPCn/YXnSJ3K3kwX7TDjH7zPX22peJpLFg2lJU0+LkB
XizOFGpGib0HJ8o2xhL/Ras4i98mGsNSVM7wcl4Swj9Z3fx2ds69kdMCgYEAyqvp
h0s50kzgvEMwYwveXo7wNjojqe/NU7FFoW02LDU/KRzTNnv0Hs0Xc3WBytqBE94U
kdITBqtzCrYBHo2JhQq4dez6H33RrgIbNSy9aGuqcblvr8gI9roH9fDeF4AqRAnZ
XRFO6IqFiSrkxJkriPt2xJR041UNFyXGG6FR6msCgYBmxlZoOmmPietpoP52yx+b
v8UDRG5ISIykevbyZk0KdFFzguUeGAcviUGCWzcQchI/NvB282vqmXVB2iu1raor
LOe2534w89oik59sItrTT/qIE/gp1aMFX15PJVbNY5Sp016GJmloQNSs0pxA4cn9
NKGexmREPD5BU7dheX87SwKBgQCVoFXIjMEjgZ5pXzFZ7mk9ZknxvvqVe3UbVMUT
aI2WFbmLoLxOfTS9iKzHkPlByg+Bm3OUNIPXaLyGK9intdbRYhjM9yeyGDG1RdjQ
aTds4A/15fGO1R/JB47ZA/rzXqvVj2/qRdz70UjE++XpPyvk9cG5X+Dr9N61OC4K
OA9CAQKBgFdDVaQEG+rERVXAgdYo0KQ741Rc3kbavdVr7tRE3eY+aJUauOcoaWjT
rEdwvajksm6a3Oft5KtsWNfXhNPACUrLxTH2wv9zoejf+q/Xyf7NYq73xWfiRChG
INX8iEaHVcH6jWNyCE/0BdPtMfvw4sypv/yxZH0RlvmhvzqemFmP
-----END RSA PRIVATE KEY-----
`
)

func TestCertificatesSecret(t *testing.T) {
	tests := []struct {
		name                                 string
		s                                    CertificatesSecret
		wantCa, wantCert, wantChain, wantKey []byte
	}{
		{
			name: "Simple chain",
			s: CertificatesSecret{
				Data: map[string][]byte{
					certificates.CAFileName:   []byte(ca),
					certificates.CertFileName: []byte(tls),
					certificates.KeyFileName:  []byte(key),
				},
			},
			wantCa:    []byte(ca),
			wantKey:   []byte(key),
			wantCert:  []byte(tls),
			wantChain: append([]byte(ca), []byte(tls)...),
		},
		{
			name: "No CA cert",
			s: CertificatesSecret{
				Data: map[string][]byte{
					certificates.CertFileName: []byte(tls),
					certificates.KeyFileName:  []byte(key),
				},
			},
			wantCa:    nil,
			wantKey:   []byte(key),
			wantCert:  []byte(tls),
			wantChain: []byte(tls),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.CertChain(); !reflect.DeepEqual(got, tt.wantChain) {
				t.Errorf("CertificatesSecret.CertChain() = %v, want %v", got, tt.wantChain)
			}
			if got := tt.s.CaPem(); !reflect.DeepEqual(got, tt.wantCa) {
				t.Errorf("CertificatesSecret.CaPem() = %v, want %v", got, tt.wantCa)
			}
			if got := tt.s.CertPem(); !reflect.DeepEqual(got, tt.wantCert) {
				t.Errorf("CertificatesSecret.CertPem() = %v, want %v", got, tt.wantCert)
			}
			if got := tt.s.KeyPem(); !reflect.DeepEqual(got, tt.wantKey) {
				t.Errorf("CertificatesSecret.CertChain() = %v, want %v", got, tt.wantKey)
			}
		})
	}
}
