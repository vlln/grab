package fingerprint

import (
	utls "github.com/refraction-networking/utls"
)

type Profile struct {
	Name    string
	ID      utls.ClientHelloID
	Referer string
}

// DefaultRotation is the ordered list of TLS fingerprint profiles to try.
// Chrome 120 is the closest available to Chrome 124 (confirmed working with bioRxiv).
// Chrome 131 and Safari 16.0 are fallback alternatives.
var DefaultRotation = []Profile{
	{Name: "chrome_120", ID: utls.HelloChrome_120, Referer: "https://www.google.com/"},
	{Name: "chrome_131", ID: utls.HelloChrome_131, Referer: "https://www.google.com/"},
	{Name: "safari_16_0", ID: utls.HelloSafari_16_0, Referer: "https://www.google.com/"},
}

// Rotation returns the ordered profiles, starting from the cached one if provided.
func Rotation(cached string) []Profile {
	profiles := DefaultRotation
	if cached == "" {
		return profiles
	}
	for i, p := range profiles {
		if p.Name == cached {
			return append([]Profile{p}, append(profiles[:i], profiles[i+1:]...)...)
		}
	}
	return profiles
}

// SpecForProfile returns a ClientHelloSpec for the given profile.
// "h2" is stripped from ALPN so the server negotiates HTTP/1.1 only.
func SpecForProfile(p Profile) (*utls.ClientHelloSpec, error) {
	spec, err := utls.UTLSIdToSpec(p.ID)
	if err != nil {
		return nil, err
	}
	// Remove "h2" from ALPN, keeping only http/1.1.
	for _, ext := range spec.Extensions {
		if alpn, ok := ext.(*utls.ALPNExtension); ok {
			filtered := alpn.AlpnProtocols[:0]
			for _, proto := range alpn.AlpnProtocols {
				if proto != "h2" {
					filtered = append(filtered, proto)
				}
			}
			alpn.AlpnProtocols = filtered
			break
		}
	}
	return &spec, nil
}