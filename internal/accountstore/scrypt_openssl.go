//go:build openssl

package accountstore

import (
	"os"
	"runtime"

	"github.com/golang-fips/openssl/v2"
	"github.com/pkg/errors"
)

func init() {
	err := openssl.Init(getVersion())
	if err != nil {
		panic(err)
	}
	_ = openssl.SetFIPS(true)
}

func scryptKey(password string, salt []byte) ([]byte, error) {
	b, err := openssl.Scrypt(
		password,
		salt,
		1<<14,
		8,
		1,
		32<<20,
		64,
	)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return b, nil
}

// getVersion returns the OpenSSL version to use for testing.
func getVersion() string {
	v := os.Getenv("GO_OPENSSL_VERSION_OVERRIDE")
	if v != "" {
		if runtime.GOOS == "linux" {
			return "libcrypto.so." + v
		}
		return v
	}
	// Try to find a supported version of OpenSSL on the system.
	// This is useful for local testing, where the user may not
	// have GO_OPENSSL_VERSION_OVERRIDE set.
	versions := []string{"3", "1.1.1", "1.1", "11", "111", "1.0.2", "1.0.0", "10"}
	if runtime.GOOS == "windows" {
		if runtime.GOARCH == "amd64" {
			versions = []string{"libcrypto-3-x64", "libcrypto-3", "libcrypto-1_1-x64", "libcrypto-1_1", "libeay64", "libeay32"}
		} else {
			versions = []string{"libcrypto-3", "libcrypto-1_1", "libeay32"}
		}
	}
	for _, v = range versions {
		if runtime.GOOS == "windows" {
			v += ".dll"
		} else if runtime.GOOS == "darwin" {
			v = "libcrypto." + v + ".dylib"
		} else {
			v = "libcrypto.so." + v
		}
		if ok, _ := openssl.CheckVersion(v); ok {
			return v
		}
	}
	return "libcrypto.so"
}
