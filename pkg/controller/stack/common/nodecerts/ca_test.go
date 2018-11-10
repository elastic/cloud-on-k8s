package nodecerts

import (
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func bigFromString(s string) *big.Int {
	ret := new(big.Int)
	ret.SetString(s, 10)
	return ret
}

// rsaPrivateKey is a private key used for testing
var rsaPrivateKey = &rsa.PrivateKey{
	PublicKey: rsa.PublicKey{
		N: bigFromString("124737666279038955318614287965056875799409043964547386061640914307192830334599556034328900586693254156136128122194531292927142396093148164407300419162827624945636708870992355233833321488652786796134504707628792159725681555822420087112284637501705261187690946267527866880072856272532711620639179596808018872997"),
		E: 65537,
	},
	D: bigFromString("69322600686866301945688231018559005300304807960033948687567105312977055197015197977971637657636780793670599180105424702854759606794705928621125408040473426339714144598640466128488132656829419518221592374964225347786430566310906679585739468938549035854760501049443920822523780156843263434219450229353270690889"),
	Primes: []*big.Int{
		bigFromString("11405025354575369741595561190164746858706645478381139288033759331174478411254205003127028642766986913445391069745480057674348716675323735886284176682955723"),
		bigFromString("10937079261204603443118731009201819560867324167189758120988909645641782263430128449826989846631183550578761324239709121189827307416350485191350050332642639"),
	},
}

var testCa *Ca

func init() {
	var err error
	testCa, err = NewSelfSignedCaUsingKey("test", rsaPrivateKey)
	if err != nil {
		panic(err)
	}
	logf.SetLogger(logf.ZapLogger(false))
}

func TestCa_CreateCertificateForValidatedCertificateTemplate(t *testing.T) {
	// create a certificate template for the csr
	cn := "test-cn"
	certificateTemplate := x509.Certificate{
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotAfter: time.Now().Add(365 * 24 * time.Hour),

		PublicKeyAlgorithm: x509.RSA,
		PublicKey:          &rsaPrivateKey.PublicKey,
	}

	bytes, err := testCa.CreateCertificateForValidatedCertificateTemplate(certificateTemplate)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(bytes)
	require.NoError(t, err)

	assert.Equal(t, cert.Subject.CommonName, cn)

	// the issued certificate should pass verification
	pool := x509.NewCertPool()
	pool.AddCert(testCa.Cert)
	_, err = cert.Verify(x509.VerifyOptions{
		DNSName: cn,
		Roots:   pool,
	})
	assert.NoError(t, err)
}
