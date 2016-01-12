package setup

import (
	"crypto/tls"
	"log"
	"strings"

	"github.com/mholt/caddy/middleware"
	"github.com/mholt/caddy/server"
)

// TLS sets up the TLS configuration (but does not activate Let's Encrypt; that is handled elsewhere).
func TLS(c *Controller) (middleware.Middleware, error) {
	if c.Scheme == "http" {
		c.TLS.Enabled = false
		log.Printf("[WARNING] TLS disabled for %s://%s.", c.Scheme, c.Address())
	} else {
		c.TLS.Enabled = true
	}

	for c.Next() {
		args := c.RemainingArgs()
		switch len(args) {
		case 1:
			c.TLS.LetsEncryptEmail = args[0]

			// user can force-disable LE activation this way
			if c.TLS.LetsEncryptEmail == "off" {
				c.TLS.Enabled = false
			}
		case 2:
			c.TLS.Certificate = args[0]
			c.TLS.Key = args[1]
		}

		// Optional block with extra parameters
		var hadBlock bool
		for c.NextBlock() {
			hadBlock = true
			switch c.Val() {
			case "protocols":
				args := c.RemainingArgs()
				if len(args) != 2 {
					return nil, c.ArgErr()
				}
				value, ok := supportedProtocols[strings.ToLower(args[0])]
				if !ok {
					return nil, c.Errf("Wrong protocol name or protocol not supported '%s'", c.Val())
				}
				c.TLS.ProtocolMinVersion = value
				value, ok = supportedProtocols[strings.ToLower(args[1])]
				if !ok {
					return nil, c.Errf("Wrong protocol name or protocol not supported '%s'", c.Val())
				}
				c.TLS.ProtocolMaxVersion = value
			case "ciphers":
				for c.NextArg() {
					value, ok := supportedCiphersMap[strings.ToUpper(c.Val())]
					if !ok {
						return nil, c.Errf("Wrong cipher name or cipher not supported '%s'", c.Val())
					}
					c.TLS.Ciphers = append(c.TLS.Ciphers, value)
				}
			case "clients":
				c.TLS.ClientCerts = c.RemainingArgs()
				if len(c.TLS.ClientCerts) == 0 {
					return nil, c.ArgErr()
				}
			// TODO: Allow this? It's a bad idea to allow HTTP. If we do this, make sure invoking tls at all (even manually) also sets up a redirect if possible?
			// case "allow_http":
			// 	c.TLS.DisableHTTPRedir = true
			default:
				return nil, c.Errf("Unknown keyword '%s'", c.Val())
			}
		}

		// tls requires at least one argument if a block is not opened
		if len(args) == 0 && !hadBlock {
			return nil, c.ArgErr()
		}
	}

	SetDefaultTLSParams(c.Config)

	return nil, nil
}

// SetDefaultTLSParams sets the default TLS cipher suites, protocol versions,
// and server preferences of a server.Config if they were not previously set
// (it does not overwrite; only fills in missing values).
func SetDefaultTLSParams(c *server.Config) {
	// If no ciphers provided, use all that Caddy supports for the protocol
	if len(c.TLS.Ciphers) == 0 {
		c.TLS.Ciphers = defaultCiphers
	}

	// Not a cipher suite, but still important for mitigating protocol downgrade attacks
	c.TLS.Ciphers = append(c.TLS.Ciphers, tls.TLS_FALLBACK_SCSV)

	// Set default protocol min and max versions - must balance compatibility and security
	if c.TLS.ProtocolMinVersion == 0 {
		c.TLS.ProtocolMinVersion = tls.VersionTLS10
	}
	if c.TLS.ProtocolMaxVersion == 0 {
		c.TLS.ProtocolMaxVersion = tls.VersionTLS12
	}

	// Prefer server cipher suites
	c.TLS.PreferServerCipherSuites = true

	// Default TLS port is 443; only use if port is not manually specified,
	// TLS is enabled, and the host is not localhost
	if c.Port == "" && c.TLS.Enabled && c.Host != "localhost" {
		c.Port = "443"
	}
}

// Map of supported protocols
// SSLv3 will be not supported in future release
// HTTP/2 only supports TLS 1.2 and higher
var supportedProtocols = map[string]uint16{
	"ssl3.0": tls.VersionSSL30,
	"tls1.0": tls.VersionTLS10,
	"tls1.1": tls.VersionTLS11,
	"tls1.2": tls.VersionTLS12,
}

// Map of supported ciphers, used only for parsing config.
//
// Note that, at time of writing, HTTP/2 blacklists 276 cipher suites,
// including all but two of the suites below (the two GCM suites).
// See https://http2.github.io/http2-spec/#BadCipherSuites
//
// TLS_FALLBACK_SCSV is not in this list because we manually ensure
// it is always added (even though it is not technically a cipher suite).
//
// This map, like any map, is NOT ORDERED. Do not range over this map.
var supportedCiphersMap = map[string]uint16{
	"ECDHE-RSA-AES128-GCM-SHA256":   tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	"ECDHE-ECDSA-AES128-GCM-SHA256": tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	"ECDHE-RSA-AES128-CBC-SHA":      tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	"ECDHE-RSA-AES256-CBC-SHA":      tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	"ECDHE-ECDSA-AES256-CBC-SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	"ECDHE-ECDSA-AES128-CBC-SHA":    tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	"RSA-AES128-CBC-SHA":            tls.TLS_RSA_WITH_AES_128_CBC_SHA,
	"RSA-AES256-CBC-SHA":            tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	"ECDHE-RSA-3DES-EDE-CBC-SHA":    tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
	"RSA-3DES-EDE-CBC-SHA":          tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
}

// List of supported cipher suites in descending order of preference.
// Ordering is very important! Getting the wrong order will break
// mainstream clients, especially with HTTP/2.
//
// Note that TLS_FALLBACK_SCSV is not in this list since it is always
// added manually.
var supportedCiphers = []uint16{
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	tls.TLS_RSA_WITH_AES_128_CBC_SHA,
	tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
	tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
}

// List of all the ciphers we want to use by default
var defaultCiphers = []uint16{
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	tls.TLS_RSA_WITH_AES_128_CBC_SHA,
}
