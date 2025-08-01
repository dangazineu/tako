# Analysis and Planning Phase

This phase focuses on understanding the problem space and creating a comprehensive implementation plan.

## 3. Background Research

- Gather comprehensive information about:
  - The specific issue requirements
  - Related previous work and dependencies
  - Integration points with existing features
  - Overall project architecture context
  - Previous Pull Requests that attempted to resolve the issue at hand, or its parent issue (if any). This would be useful to understand paths that didn't work, or previous challenges to avoid.
- Document findings in `issue_background.md`
- Commit background analysis

## 4. Question Formulation and Resolution

- List all technical questions and uncertainties
- Ask Gemini to evaluate code, designs, issue background, and your questions
- Review Gemini's suggestions but validate against your understanding
- Document your decisions and possibly new questions in the issue background doc
- Repeat this interaction as many times as needed until you and Gemini agree on a path forward and you have no more questions
- Commit updated background analysis

## 5. Implementation Planning

- Create detailed plan with discrete phases in `issue_plan.md`
- Each phase must leave codebase in healthy state (compiling + passing tests)
- Include both implementation and testing components for each phase
- Commit the plan

## 6. Plan Review and Refinement

- Ask Gemini to critique the plan, providing:
  - Issue background documentation
  - Relevant code and design docs
  - The implementation plan
- Address Gemini's questions and integrate feedback
- Update plan and background as needed, repeat this step as many times as needed, until you have no questions left and you and Gemini agree on the plan
- Commit refined plan

## Key Requirements

- Thorough research prevents costly mistakes during implementation
- Clear planning ensures each development phase is actionable
- External review (Gemini) helps identify blind spots early
- **Failure Path**: If the analysis phase reveals the issue is infeasible as-is, update the GitHub issue with your findings and ask for clarification before proceeding