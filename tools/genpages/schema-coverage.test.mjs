import assert from "node:assert/strict";
import test from "node:test";

import { documentedYAMLSchemaFields } from "./schema-coverage.mjs";

test("excludes temporary v1 migration fields from documentation coverage", () => {
  const schema = `
type AgentSpec struct {
  Name        string \`yaml:"name,omitempty"\`
  DisplayName string \`yaml:"display_name,omitempty"\`
  Description string \`yaml:"description,omitempty"\`
}

type WorkspaceSpec struct {
  Commit      string \`yaml:"commit,omitempty"\`
  Internal    string \`yaml:"-"\`
}

type ReleaseSpec struct {
  Commit string \`yaml:"commit,omitempty"\`
}
`;

  assert.deepEqual(
    [...documentedYAMLSchemaFields(schema)].sort(),
    ["commit", "name"],
  );
});
