// Copyright © 2017-2019 Aqua Security Software Ltd. <info@aquasec.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package check

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gopkg.in/yaml.v2"
)

const cfgDir = "../cfg/"

type mockRunner struct {
	mock.Mock
}

func (m *mockRunner) Run(c *Check) State {
	args := m.Called(c)
	return args.Get(0).(State)
}

// validate that the files we're shipping are valid YAML
func TestYamlFiles(t *testing.T) {
	err := filepath.Walk(cfgDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			t.Fatalf("failure accessing path %q: %v\n", path, err)
		}
		if !info.IsDir() {
			t.Logf("reading file: %s", path)
			in, err := ioutil.ReadFile(path)
			if err != nil {
				t.Fatalf("error opening file %s: %v", path, err)
			}

			c := new(Controls)
			err = yaml.Unmarshal(in, c)
			if err == nil {
				t.Logf("YAML file successfully unmarshalled: %s", path)
			} else {
				t.Fatalf("failed to load YAML from %s: %v", path, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failure walking cfg dir: %v\n", err)
	}
}

func TestNewControls(t *testing.T) {

	t.Run("Should return error when node type is not specified", func(t *testing.T) {
		// given
		in := []byte(`
---
controls:
type: # not specified
groups:
`)
		// when
		_, err := NewControls(MASTER, in)
		// then
		assert.EqualError(t, err, "non-master controls file specified")
	})

	t.Run("Should return error when input YAML is invalid", func(t *testing.T) {
		// given
		in := []byte("BOOM")
		// when
		_, err := NewControls(MASTER, in)
		// then
		assert.EqualError(t, err, "failed to unmarshal YAML: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `BOOM` into check.Controls")
	})

}

func TestControls_RunChecks(t *testing.T) {

	t.Run("Should run checks matching the filter and update summaries", func(t *testing.T) {
		// given
		runner := new(mockRunner)
		// and
		in := []byte(`
---
type: "master"
groups:
- id: G1
  checks:
  - id: G1/C1
- id: G2
  checks:
  - id: G2/C1
    text: "Verify that the SomeSampleFlag argument is set to true"
    audit: "grep -B1 SomeSampleFlag=true /this/is/a/file/path"
    tests:
      test_items:
      - flag: "SomeSampleFlag=true"
        compare:
          op: has
          value: "true"
        set: true
    remediation: |
      Edit the config file /this/is/a/file/path and set SomeSampleFlag to true.
    scored: true
`)
		// and
		controls, err := NewControls(MASTER, in)
		assert.NoError(t, err)
		// and
		runner.On("Run", controls.Groups[0].Checks[0]).Return(PASS)
		runner.On("Run", controls.Groups[1].Checks[0]).Return(FAIL)
		// and
		var runAll Predicate = func(group *Group, c *Check) bool {
			return true
		}
		// when
		controls.RunChecks(runner, runAll)
		// then
		assert.Equal(t, 2, len(controls.Groups))
		// and
		G1 := controls.Groups[0]
		assert.Equal(t, "G1", G1.ID)
		assert.Equal(t, "G1/C1", G1.Checks[0].ID)
		assertEqualGroupSummary(t, 1, 0, 0, 0, G1)
		// and
		G2 := controls.Groups[1]
		assert.Equal(t, "G2", G2.ID)
		assert.Equal(t, "G2/C1", G2.Checks[0].ID)
		assert.Equal(t, "has", G2.Checks[0].Tests.TestItems[0].Compare.Op)
		assert.Equal(t, "true", G2.Checks[0].Tests.TestItems[0].Compare.Value)
		assert.Equal(t, true, G2.Checks[0].Tests.TestItems[0].Set)
		assert.Equal(t, "SomeSampleFlag=true", G2.Checks[0].Tests.TestItems[0].Flag)
		assert.Equal(t, "Edit the config file /this/is/a/file/path and set SomeSampleFlag to true.\n", G2.Checks[0].Remediation)
		assert.Equal(t, true, G2.Checks[0].Scored)
		assertEqualGroupSummary(t, 0, 1, 0, 0, G2)
		// and
		assert.Equal(t, 1, controls.Summary.Pass)
		assert.Equal(t, 1, controls.Summary.Fail)
		assert.Equal(t, 0, controls.Summary.Info)
		assert.Equal(t, 0, controls.Summary.Warn)
		// and
		runner.AssertExpectations(t)
	})
}

func assertEqualGroupSummary(t *testing.T, pass, fail, info, warn int, actual *Group) {
	t.Helper()
	assert.Equal(t, pass, actual.Pass)
	assert.Equal(t, fail, actual.Fail)
	assert.Equal(t, info, actual.Info)
	assert.Equal(t, warn, actual.Warn)
}
