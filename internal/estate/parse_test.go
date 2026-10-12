package estate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseClusters(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		exp    []Cluster
		expErr string
	}{
		{
			name:  "without windows",
			input: "name,provisioned_vcpu,peak_vcpu\na,4,1\nb,12,5\n",
			exp: []Cluster{
				{Name: "a", Provisioned: 4, Peak: 1},
				{Name: "b", Provisioned: 12, Peak: 5},
			},
		},
		{
			name:  "with windows",
			input: "name,provisioned_vcpu,peak_vcpu,base_vcpu,peak_from,peak_to\na,12,10,2,9,17\n",
			exp:   []Cluster{{Name: "a", Provisioned: 12, Peak: 10, Base: 2, PeakFrom: 9, PeakTo: 17, Windowed: true}},
		},
		{
			name:  "peak_to of 24 normalises to midnight",
			input: "name,provisioned_vcpu,peak_vcpu,peak_from,peak_to\na,12,10,9,24\n",
			exp:   []Cluster{{Name: "a", Provisioned: 12, Peak: 10, PeakFrom: 9, PeakTo: 0, Windowed: true}},
		},
		{
			name:  "columns in any order",
			input: "peak_vcpu,provisioned_vcpu,name\n1,4,a\n",
			exp:   []Cluster{{Name: "a", Provisioned: 4, Peak: 1}},
		},
		{
			name:  "header casing and padding are ignored",
			input: " Name , Provisioned_vCPU , PEAK_VCPU \na, 4, 1\n",
			exp:   []Cluster{{Name: "a", Provisioned: 4, Peak: 1}},
		},
		{
			name:  "a byte order mark is ignored",
			input: "\ufeffname,provisioned_vcpu,peak_vcpu\na,4,1\n",
			exp:   []Cluster{{Name: "a", Provisioned: 4, Peak: 1}},
		},
		{
			name:  "a blank base_vcpu defaults to zero",
			input: "name,provisioned_vcpu,peak_vcpu,base_vcpu\na,4,1,\n",
			exp:   []Cluster{{Name: "a", Provisioned: 4, Peak: 1}},
		},
		{
			name:   "missing name column",
			input:  "provisioned_vcpu,peak_vcpu\n4,1\n",
			expErr: `missing "name" column`,
		},
		{
			name:   "blank name",
			input:  "name,provisioned_vcpu,peak_vcpu\n,4,1\n",
			expErr: "line 2: missing name",
		},
		{
			name:   "duplicate name",
			input:  "name,provisioned_vcpu,peak_vcpu\na,4,1\na,8,2\n",
			expErr: `line 3: duplicate name "a", first seen on line 2`,
		},
		{
			name:   "missing column",
			input:  "name,provisioned_vcpu\na,4\n",
			expErr: `missing "peak_vcpu" column`,
		},
		{
			name:   "duplicate column",
			input:  "name,peak_vcpu,provisioned_vcpu,peak_vcpu\na,1,4,999\n",
			expErr: `duplicate "peak_vcpu" column`,
		},
		{
			name:   "not a number",
			input:  "name,provisioned_vcpu,peak_vcpu\na,4,NaN\n",
			expErr: "line 2: peak_vcpu must be a finite number",
		},
		{
			name:   "infinite",
			input:  "name,provisioned_vcpu,peak_vcpu\na,Inf,1\n",
			expErr: "line 2: provisioned_vcpu must be a finite number",
		},
		{
			name:   "negative infinity",
			input:  "name,provisioned_vcpu,peak_vcpu\na,4,-Inf\n",
			expErr: "line 2: peak_vcpu must be a finite number",
		},
		{
			name:   "peak_from without peak_to",
			input:  "name,provisioned_vcpu,peak_vcpu,peak_from\na,4,1,9\n",
			expErr: `"peak_from" and "peak_to" must be provided together`,
		},
		{
			name:   "hour out of range",
			input:  "name,provisioned_vcpu,peak_vcpu,peak_from,peak_to\na,4,1,25,9\n",
			expErr: "line 2: peak_from 25 is out of range, expected an hour between 0 and 23",
		},
		{
			name:   "non numeric hour",
			input:  "name,provisioned_vcpu,peak_vcpu,peak_from,peak_to\na,4,1,nine,17\n",
			expErr: `line 2: invalid peak_from "nine", expected an hour between 0 and 23`,
		},
		{
			name:   "missing hour",
			input:  "name,provisioned_vcpu,peak_vcpu,peak_from,peak_to\na,4,1,,17\n",
			expErr: "line 2: missing peak_from",
		},
		{
			name:   "base above peak",
			input:  "name,provisioned_vcpu,peak_vcpu,base_vcpu\na,10,4,5\n",
			expErr: "line 2: base_vcpu (5) exceeds peak_vcpu (4)",
		},
		{
			name:   "peak above provisioned",
			input:  "name,provisioned_vcpu,peak_vcpu\na,4,5\n",
			expErr: "line 2: peak_vcpu (5) exceeds provisioned_vcpu (4)",
		},
		{
			name:   "negative vcpu",
			input:  "name,provisioned_vcpu,peak_vcpu\na,-4,1\n",
			expErr: "line 2: provisioned_vcpu must not be negative",
		},
		{
			name:   "invalid number",
			input:  "name,provisioned_vcpu,peak_vcpu\na,4,abc\n",
			expErr: `line 2: invalid peak_vcpu "abc"`,
		},
		{
			name:   "short row",
			input:  "name,provisioned_vcpu,peak_vcpu\na,4\n",
			expErr: "line 2: missing peak_vcpu",
		},
		{
			name:   "no rows",
			input:  "name,provisioned_vcpu,peak_vcpu\n",
			expErr: "no clusters found",
		},
		{
			name:   "empty file",
			input:  "",
			expErr: "file is empty",
		},
		{
			name:   "unparseable csv",
			input:  "name,provisioned_vcpu,peak_vcpu\na,4,\"1\n",
			expErr: `reading line 2: parse error on line 2, column 8: extraneous or missing " in quoted-field`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			act, err := ParseClusters(strings.NewReader(c.input))

			if c.expErr != "" {
				if err == nil || err.Error() != c.expErr {
					t.Fatalf("expected error %q, got %v", c.expErr, err)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(act) != len(c.exp) {
				t.Fatalf("expected %d clusters, got %d", len(c.exp), len(act))
			}

			for i := range act {
				if act[i] != c.exp[i] {
					t.Fatalf("cluster %d: expected %+v, got %+v", i, c.exp[i], act[i])
				}
			}
		})
	}
}

func TestReadClusters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clusters.csv")
	if err := os.WriteFile(path, []byte("name,provisioned_vcpu,peak_vcpu\na,4,1\n"), 0o600); err != nil {
		t.Fatalf("writing the config: %v", err)
	}

	clusters, err := ReadClusters(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if exp := (Cluster{Name: "a", Provisioned: 4, Peak: 1}); len(clusters) != 1 || clusters[0] != exp {
		t.Fatalf("expected %+v, got %+v", exp, clusters)
	}

	if _, err := ReadClusters(filepath.Join(t.TempDir(), "missing.csv")); err == nil {
		t.Fatal("expected an error for a file that isn't there")
	} else if !strings.HasPrefix(err.Error(), "opening file: ") {
		t.Errorf("expected the error to say which step failed, got %v", err)
	}
}
