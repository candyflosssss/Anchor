package updater

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z.-]+))?$`)

type version struct {
	numbers [3]uint64
	pre     []string
}

func parseVersion(s string) (version, error) {
	var v version
	m := versionPattern.FindStringSubmatch(s)
	if m == nil {
		return v, fmt.Errorf("%w: invalid version", ErrInvalid)
	}
	for i := range v.numbers {
		n, err := strconv.ParseUint(m[i+1], 10, 32)
		if err != nil {
			return v, ErrInvalid
		}
		v.numbers[i] = n
	}
	if m[4] != "" {
		v.pre = strings.Split(m[4], ".")
		for _, p := range v.pre {
			if p == "" {
				return v, ErrInvalid
			}
			if numeric(p) && len(p) > 1 && p[0] == '0' {
				return v, ErrInvalid
			}
		}
	}
	return v, nil
}
func numeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}
func compareVersions(a, b string) (int, error) {
	x, err := parseVersion(a)
	if err != nil {
		return 0, err
	}
	y, err := parseVersion(b)
	if err != nil {
		return 0, err
	}
	for i, n := range x.numbers {
		if n > y.numbers[i] {
			return 1, nil
		}
		if n < y.numbers[i] {
			return -1, nil
		}
	}
	if len(x.pre) == 0 && len(y.pre) == 0 {
		return 0, nil
	}
	if len(x.pre) == 0 {
		return 1, nil
	}
	if len(y.pre) == 0 {
		return -1, nil
	}
	for i := 0; i < len(x.pre) && i < len(y.pre); i++ {
		p, q := x.pre[i], y.pre[i]
		if p == q {
			continue
		}
		pn, qn := numeric(p), numeric(q)
		if pn && !qn {
			return -1, nil
		}
		if !pn && qn {
			return 1, nil
		}
		if pn && len(p) != len(q) {
			if len(p) > len(q) {
				return 1, nil
			}
			return -1, nil
		}
		if p > q {
			return 1, nil
		}
		return -1, nil
	}
	if len(x.pre) > len(y.pre) {
		return 1, nil
	}
	if len(x.pre) < len(y.pre) {
		return -1, nil
	}
	return 0, nil
}
func versionChannel(s string) (string, error) {
	v, err := parseVersion(s)
	if err != nil {
		return "", err
	}
	if len(v.pre) == 0 {
		return "stable", nil
	}
	c := v.pre[0]
	if !ValidChannel(c) {
		return "", ErrInvalid
	}
	return c, nil
}
func ValidChannel(c string) bool {
	return c == "stable" || c == "alpha" || c == "beta" || c == "rc" || c == "dev"
}
