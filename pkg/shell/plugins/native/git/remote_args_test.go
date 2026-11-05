package git

import (
	"reflect"
	"testing"

	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

func TestParsePushArgs(t *testing.T) {
	cases := []struct {
		args        []string
		remote      string
		specs       []config.RefSpec
		force, tags bool
	}{
		{nil, "origin", nil, false, false},
		{[]string{"origin", "master"}, "origin", []config.RefSpec{"refs/heads/master:refs/heads/master"}, false, false},
		{[]string{"-f", "upstream", "main", "dev"}, "upstream",
			[]config.RefSpec{"refs/heads/main:refs/heads/main", "refs/heads/dev:refs/heads/dev"}, true, false},
		{[]string{"--tags", "origin", "+refs/heads/a:refs/heads/b"}, "origin", []config.RefSpec{"+refs/heads/a:refs/heads/b"}, false, true},
	}
	for _, c := range cases {
		remote, specs, force, tags := parsePushArgs(c.args)
		if remote != c.remote || !reflect.DeepEqual(specs, c.specs) || force != c.force || tags != c.tags {
			t.Errorf("parsePushArgs(%q) = %q %v %v %v; want %q %v %v %v",
				c.args, remote, specs, force, tags, c.remote, c.specs, c.force, c.tags)
		}
	}
}

func TestParsePullArgs(t *testing.T) {
	if r, b := parsePullArgs(nil); r != "origin" || b != "" {
		t.Errorf("no args = %q %q", r, b)
	}
	if r, b := parsePullArgs([]string{"origin", "main"}); r != "origin" || b != plumbing.NewBranchReferenceName("main") {
		t.Errorf("origin main = %q %q", r, b)
	}
	if r, b := parsePullArgs([]string{"--rebase", "upstream"}); r != "upstream" || b != "" {
		t.Errorf("--rebase upstream = %q %q", r, b)
	}
}
