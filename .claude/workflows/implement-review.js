export const meta = {
  name: 'implement-review',
  description: 'Implement one work package, review it through three lenses, adversarially verify each finding, and fix what survives',
  whenToUse: 'A single, well-scoped documango change with known file ownership. Pass {task, owns, checks}. The main session commits afterwards.',
  phases: [
    { title: 'Implement', detail: 'implementer builds the package test first' },
    { title: 'Review', detail: 'correctness, tests, and user-facing lenses' },
    { title: 'Verify', detail: 'adversarial reviewer tries to refute each finding' },
    { title: 'Fix', detail: 'implementer fixes confirmed findings, up to two rounds' },
  ],
}

const task = args && args.task
if (!task) throw new Error('implement-review needs args.task')
const owns = (args.owns || []).join(', ') || 'the files the task names'
const checks = args.checks || 'go test -race ./...'

const REPORT = {
  type: 'object',
  properties: {
    summary: { type: 'string' },
    filesChanged: { type: 'array', items: { type: 'string' } },
    checksRun: { type: 'string', description: 'commands run and their results' },
    openIssues: { type: 'array', items: { type: 'string' } },
  },
  required: ['summary', 'filesChanged', 'checksRun', 'openIssues'],
}

const FINDINGS = {
  type: 'object',
  properties: {
    findings: {
      type: 'array',
      items: {
        type: 'object',
        properties: {
          file: { type: 'string' },
          line: { type: 'integer' },
          summary: { type: 'string' },
          failure: { type: 'string', description: 'input or state that triggers it and what goes wrong' },
        },
        required: ['file', 'summary', 'failure'],
      },
    },
  },
  required: ['findings'],
}

const VERDICT = {
  type: 'object',
  properties: {
    refuted: { type: 'boolean' },
    evidence: { type: 'string', description: 'what was run or read, and what it showed' },
  },
  required: ['refuted', 'evidence'],
}

const LENSES = [
  'correctness: logic errors, edge cases, error handling, concurrency, security of file paths and network input',
  'tests: behaviour the change adds or alters that no test covers, tests that would pass even if the code were wrong, flaky timing',
  'user-facing behaviour: CLI output and exit codes, help text, rendered HTML and CSS, accessibility, docs that no longer match the code',
]

phase('Implement')
let report = await agent(
  `Implement this documango work package.\n\nTask:\n${task}\n\nFiles you own: ${owns}\nChecks to run before finishing: ${checks}`,
  { label: 'implement', agentType: 'implementer', schema: REPORT },
)

async function review(round) {
  const changed = report.filesChanged.join(', ')
  const found = await parallel(LENSES.map((lens, i) => () =>
    agent(
      `Review this documango change through the lens of ${lens}.\n\nTask it implements:\n${task}\n\nFiles changed: ${changed}\nUse git diff to see the change.`,
      { label: `review:${i + 1}:r${round}`, phase: 'Review', agentType: 'reviewer', schema: FINDINGS },
    )))
  const findings = found.filter(Boolean).flatMap(r => r.findings)
  log(`round ${round}: ${findings.length} findings to verify`)
  const verdicts = await parallel(findings.map((f, i) => () =>
    agent(
      `Try to refute this review finding in documango.\n\n${f.file}${f.line ? ':' + f.line : ''}: ${f.summary}\nClaimed failure: ${f.failure}`,
      { label: `verify:${i + 1}:r${round}`, phase: 'Verify', agentType: 'adversarial-reviewer', schema: VERDICT },
    ).then(v => ({ ...f, verdict: v }))))
  return verdicts.filter(Boolean).filter(f => f.verdict && !f.verdict.refuted)
}

let confirmed = await review(1)
let round = 1
while (confirmed.length && round <= 2) {
  phase('Fix')
  const list = confirmed.map(f => `- ${f.file}${f.line ? ':' + f.line : ''}: ${f.summary}\n  Failure: ${f.failure}\n  Evidence: ${f.verdict.evidence}`).join('\n')
  report = await agent(
    `Fix these confirmed defects in your documango work package. Add a test for each that fails before the fix.\n\nTask:\n${task}\n\nFiles you own: ${owns}\nChecks to run: ${checks}\n\nDefects:\n${list}`,
    { label: `fix:r${round}`, agentType: 'implementer', schema: REPORT },
  )
  round++
  confirmed = await review(round)
}
if (confirmed.length) log(`${confirmed.length} confirmed findings remain after two fix rounds`)

return { report, unresolved: confirmed }
