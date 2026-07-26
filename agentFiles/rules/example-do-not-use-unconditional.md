# Universal Testing Requirements

## Framework
* Use Vitest for all unit and integration tests.

## Conventions
* Test files must sit right next to the source file they target.
* Name files using the `.test.ts` or `.spec.ts` naming format.

## Execution Rules
* Before declaring a task finished, run `npm test` using your terminal tool.
* Do not call live external APIs during testing; mock network responses using MSW.
