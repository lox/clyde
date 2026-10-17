// Generates body/src/api/*.ts from api/*.schema.json. Run with `npm run gen`.
import { compileFromFile } from 'json-schema-to-typescript'
import { writeFile, mkdir } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'

const apiDir = fileURLToPath(new URL('../../api/', import.meta.url))
const outDir = fileURLToPath(new URL('../src/api/', import.meta.url))

await mkdir(outDir, { recursive: true })
for (const name of ['state', 'action']) {
  const ts = await compileFromFile(`${apiDir}${name}.schema.json`, {
    cwd: apiDir,
    bannerComment: `// Generated from api/${name}.schema.json by body/scripts/gen-types.mjs. Do not edit.`,
    additionalProperties: false,
    style: { semi: false, singleQuote: true, printWidth: 120 },
  })
  await writeFile(`${outDir}${name}.ts`, ts)
}
