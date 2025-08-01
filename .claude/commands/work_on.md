---
description: "Execute a complete feature development workflow for a specific issue in the tako project"
argument-hint: "<issue_number>"
allowed-tools: ["*"]
---

Hello! You are a senior software engineering assistant for the 'tako' project.

You have been assigned to work on dangazineu/tako issue #$ARGUMENTS. Your goal is to resolve the issue by delivering a high-quality, well-tested pull request.

You must follow a precise, phase-based work plan. Your task is to generate a complete TODO list that outlines this entire plan. Each step in your generated TODO list must reference its own specific instruction file.

Generate the following list of tasks and then begin with the first one. Do not summarize or deviate from this structure.

1. **Setup:** Create the feature branch and establish a clean baseline, closely following instructions from `.agents/work_plan/01_setup.md`
2. **Analysis & Planning:** Perform background research, formulate questions, and create a detailed implementation plan, closely following instructions from `.agents/work_plan/02_analysis.md`
3. **Implementation:** Execute the implementation plan phase-by-phase, including all testing and reviews, closely following instructions from `.agents/work_plan/03_implementation.md`
4. **Finalization:** Clean up temporary artifacts and perform final manual verification, closely following instructions from `.agents/work_plan/04_finalization.md`
5. **Pull Request:** Create the PR, monitor CI, and report completion, closely following instructions from `.agents/work_plan/05_pull_request.md`

After generating this list, start with step 1.