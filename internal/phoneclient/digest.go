package phoneclient

import (
	"crypto/md5" // #nosec G501 -- RFC 7616 compatibility with Poly Edge's MD5 challenge.
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"net/http"
	"strings"
)

func digestAuthorization(challenge string, request *http.Request, username, password string) (string, error) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(challenge)), "digest ") {
		return "", errors.New("phone did not provide an HTTP Digest challenge")
	}
	values := parseDigestValues(strings.TrimSpace(challenge[len("Digest "):]))
	realm, nonce := values["realm"], values["nonce"]
	if realm == "" || nonce == "" {
		return "", errors.New("phone provided an incomplete HTTP Digest challenge")
	}
	algorithm := strings.ToUpper(values["algorithm"])
	if algorithm == "" {
		algorithm = "MD5"
	}
	var newHash func() hash.Hash
	switch algorithm {
	case "MD5":
		newHash = md5.New // #nosec G401 -- protocol compatibility, never used for storage.
	case "SHA-256":
		newHash = sha256.New
	default:
		return "", fmt.Errorf("unsupported phone HTTP Digest algorithm %q", algorithm)
	}
	digest := func(value string) string {
		h := newHash()
		_, _ = h.Write([]byte(value))
		return hex.EncodeToString(h.Sum(nil))
	}
	uri := request.URL.RequestURI()
	ha1 := digest(username + ":" + realm + ":" + password)
	ha2 := digest(request.Method + ":" + uri)
	qop := selectQOP(values["qop"])
	parts := []string{
		`username="` + escapeDigest(username) + `"`,
		`realm="` + escapeDigest(realm) + `"`,
		`nonce="` + escapeDigest(nonce) + `"`,
		`uri="` + escapeDigest(uri) + `"`,
	}
	if opaque := values["opaque"]; opaque != "" {
		parts = append(parts, `opaque="`+escapeDigest(opaque)+`"`)
	}
	if qop == "" {
		parts = append(parts, "response=\""+digest(ha1+":"+nonce+":"+ha2)+"\"")
	} else {
		cnonce, err := randomHex(16)
		if err != nil {
			return "", fmt.Errorf("generate HTTP Digest nonce: %w", err)
		}
		nc := "00000001"
		response := digest(strings.Join([]string{ha1, nonce, nc, cnonce, qop, ha2}, ":"))
		parts = append(parts, "response=\""+response+"\"", "algorithm="+algorithm, "qop="+qop, "nc="+nc, `cnonce="`+cnonce+`"`)
		return "Digest " + strings.Join(parts, ", "), nil
	}
	parts = append(parts, "algorithm="+algorithm)
	return "Digest " + strings.Join(parts, ", "), nil
}

func selectQOP(value string) string {
	for _, candidate := range strings.Split(value, ",") {
		if strings.TrimSpace(candidate) == "auth" {
			return "auth"
		}
	}
	return ""
}

func parseDigestValues(value string) map[string]string {
	values := make(map[string]string)
	for len(value) > 0 {
		value = strings.TrimLeft(value, " ,\t")
		equal := strings.IndexByte(value, '=')
		if equal < 1 {
			break
		}
		key := strings.ToLower(strings.TrimSpace(value[:equal]))
		value = strings.TrimSpace(value[equal+1:])
		if value == "" {
			break
		}
		var result string
		if value[0] == '"' {
			value = value[1:]
			var escaped bool
			var index int
			for index = 0; index < len(value); index++ {
				if escaped {
					result += string(value[index])
					escaped = false
					continue
				}
				if value[index] == '\\' {
					escaped = true
					continue
				}
				if value[index] == '"' {
					break
				}
				result += string(value[index])
			}
			if index == len(value) {
				break
			}
			value = value[index+1:]
		} else {
			index := strings.IndexByte(value, ',')
			if index < 0 {
				result, value = strings.TrimSpace(value), ""
			} else {
				result, value = strings.TrimSpace(value[:index]), value[index:]
			}
		}
		values[key] = result
	}
	return values
}

func escapeDigest(value string) string {
	return strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(value)
}
