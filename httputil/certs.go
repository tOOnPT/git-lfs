package httputil

import (
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/github/git-lfs/config"
	"github.com/rubyist/tracerx"
)

// isCertVerificationDisabledForHost returns whether SSL certificate verification
// has been disabled for the given host, or globally
func isCertVerificationDisabledForHost(cfg *config.Configuration, host string) bool {
	hostSslVerify, _ := cfg.Git.Get(fmt.Sprintf("http.https://%v/.sslverify", host))
	if hostSslVerify == "false" {
		return true
	}

	globalSslVerify, _ := cfg.Git.Get("http.sslverify")
	if globalSslVerify == "false" || cfg.Os.Bool("GIT_SSL_NO_VERIFY", false) {
		return true
	}

	return false

}

// getRootCAsForHost returns a certificate pool for that specific host (which may
// be "host:port" loaded from either the gitconfig or from a platform-specific
// source which is not included by default in the golang certificate search)
// May return nil if it doesn't have anything to add, in which case the default
// RootCAs will be used if passed to TLSClientConfig.RootCAs
func getRootCAsForHost(cfg *config.Configuration, host string) *x509.CertPool {

	// don't init pool, want to return nil not empty if none found; init only on successful add cert
	var pool *x509.CertPool

	// gitconfig first
	pool = appendRootCAsForHostFromGitconfig(cfg, pool, host)
	// Platform specific
	return appendRootCAsForHostFromPlatform(pool, host)

}

func appendRootCAsForHostFromGitconfig(cfg *config.Configuration, pool *x509.CertPool, host string) *x509.CertPool {
	// Accumulate certs from all these locations:

	// GIT_SSL_CAINFO first
	if cafile, _ := cfg.Os.Get("GIT_SSL_CAINFO"); len(cafile) > 0 {
		return appendCertsFromFile(pool, cafile)
	}
	// http.<url>/.sslcainfo or http.<url>.sslcainfo
	// we know we have simply "host" or "host:port"
	hostKeyWithSlash := fmt.Sprintf("http.https://%v/.sslcainfo", host)
	if cafile, ok := cfg.Git.Get(hostKeyWithSlash); ok {
		return appendCertsFromFile(pool, cafile)
	}
	hostKeyWithoutSlash := fmt.Sprintf("http.https://%v.sslcainfo", host)
	if cafile, ok := cfg.Git.Get(hostKeyWithoutSlash); ok {
		return appendCertsFromFile(pool, cafile)
	}
	// http.sslcainfo
	if cafile, ok := cfg.Git.Get("http.sslcainfo"); ok {
		return appendCertsFromFile(pool, cafile)
	}
	// GIT_SSL_CAPATH
	if cadir, _ := cfg.Os.Get("GIT_SSL_CAPATH"); len(cadir) > 0 {
		return appendCertsFromFilesInDir(pool, cadir)
	}
	// http.sslcapath
	if cadir, ok := cfg.Git.Get("http.sslcapath"); ok {
		return appendCertsFromFilesInDir(pool, cadir)
	}

	return pool

}

func appendCertsFromFilesInDir(pool *x509.CertPool, dir string) *x509.CertPool {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		tracerx.Printf("Error reading cert dir %q: %v", dir, err)
		return pool
	}
	for _, f := range files {
		pool = appendCertsFromFile(pool, filepath.Join(dir, f.Name()))
	}
	return pool
}

func appendCertsFromFile(pool *x509.CertPool, filename string) *x509.CertPool {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		tracerx.Printf("Error reading cert file %q: %v", filename, err)
		return pool
	}
	// Firstly, try parsing as binary certificate
	if certs, err := x509.ParseCertificates(data); err == nil {
		return appendCerts(pool, certs)
	}
	// If not binary certs, try PEM data
	return appendCertsFromPEMData(pool, data)
}

func appendCerts(pool *x509.CertPool, certs []*x509.Certificate) *x509.CertPool {
	if len(certs) == 0 {
		// important to return unmodified (may be nil)
		return pool
	}

	if pool == nil {
		pool = x509.NewCertPool()
	}

	for _, cert := range certs {
		pool.AddCert(cert)
	}

	return pool
}
func appendCertsFromPEMData(pool *x509.CertPool, data []byte) *x509.CertPool {
	if len(data) == 0 {
		return pool
	}

	// Bit of a dance, need to ensure if AppendCertsFromPEM fails we still return
	// nil and not an empty pool, so system roots still get used
	var ret *x509.CertPool
	if pool == nil {
		ret = x509.NewCertPool()
	} else {
		ret = pool
	}
	if !ret.AppendCertsFromPEM(data) {
		// Return unmodified input pool (may be nil, do not replace with empty)
		return pool
	}
	return ret

}
