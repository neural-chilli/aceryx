package plugins

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var semverRe = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:[-+].*)?$`)

type semver struct {
	Major int
	Minor int
	Patch int
}

func parseSemver(v string) (semver, error) {
	m := semverRe.FindStringSubmatch(strings.TrimSpace(v))
	if len(m) != 4 {
		return semver{}, fmt.Errorf("invalid semver: %s", v)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	return semver{Major: major, Minor: minor, Patch: patch}, nil
}

func compareSemver(a, b string) (int, error) {
	sa, err := parseSemver(a)
	if err != nil {
		return 0, err
	}
	sb, err := parseSemver(b)
	if err != nil {
		return 0, err
	}
	if sa.Major != sb.Major {
		if sa.Major < sb.Major {
			return -1, nil
		}
		return 1, nil
	}
	if sa.Minor != sb.Minor {
		if sa.Minor < sb.Minor {
			return -1, nil
		}
		return 1, nil
	}
	if sa.Patch != sb.Patch {
		if sa.Patch < sb.Patch {
			return -1, nil
		}
		return 1, nil
	}
	return 0, nil
}

func sortPluginsByVersionDesc(items []*Plugin) {
	sort.Slice(items, func(i, j int) bool {
		cmp, err := compareSemver(items[i].Version, items[j].Version)
		if err != nil {
			return items[i].Version > items[j].Version
		}
		return cmp > 0
	})
}
