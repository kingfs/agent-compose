// These fields preserve v1 presentation metadata during migration. They are
// intentionally absent from the v2 authoring manual and will leave with the
// compatibility path.
const v1MigrationCompatibilityFields = new Set([
  "AgentSpec.description",
  "AgentSpec.display_name",
  "SchedulerSpec.description",
  "SchedulerSpec.display_name",
  "WorkspaceSpec.commit",
]);

export function documentedYAMLSchemaFields(schema) {
  const fields = new Set();
  for (const definition of schema.matchAll(/type\s+(\w+)\s+struct\s*\{([\s\S]*?)\n\}/g)) {
    const [, typeName, body] = definition;
    for (const tag of body.matchAll(/yaml:"([^",]+)/g)) {
      const field = tag[1];
      if (field !== "-" && !v1MigrationCompatibilityFields.has(`${typeName}.${field}`)) {
        fields.add(field);
      }
    }
  }
  return fields;
}
