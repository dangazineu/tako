# E2E Test: Java Binary Incompatibility

This test validates `tako`'s ability to correctly identify and rebuild a dependency graph in response to a subtle Java binary incompatibility.

## Scenario

The test environment consists of three Java Maven projects organized as repositories:

-   `repo-a`: A base library.
-   `repo-b`: A library that depends on `repo-a`.
-   `repo-c`: A final application that has a test suite depending on `repo-b`.

The dependency chain is `repo-c` -> `repo-b` -> `repo-a`.

## Test Flow

1.  **Initial Clean Build:** The test begins by running `tako run mvn clean install`. `tako` correctly builds the projects in the order `repo-a`, `repo-b`, then `repo-c`, and all tests in `repo-c` pass.

2.  **Introducing a Binary Incompatibility:** The test then introduces a breaking change in `repo-a` that is a **binary incompatibility**, not a source incompatibility. Specifically, it changes a public method in a class from being an instance method to a `static` method.
    -   `repo-a` can be recompiled successfully on its own, as no code within it is affected by this change.

3.  **Simulating Naive Rebuild:** The test simulates a developer mistake by rebuilding only `repo-a` using a direct `mvn install` command.

4.  **Verifying Downstream Failure:** The test suite in `repo-c` is run again. At this point, the test is expected to **fail** with a `java.lang.IncompatibleClassChangeError`. This happens because:
    -   `repo-b` was compiled against the *old* `repo-a` artifact, where the method was an instance method.
    -   The test in `repo-c` runs with the *new* `repo-a` artifact, where the method is now `static`.
    -   The JVM's method linkage fails at runtime, proving that a simple, partial rebuild is insufficient.

5.  **The `tako` Solution:** The test then executes `tako run "mvn clean install"`. `tako` analyzes the dependency graph and correctly builds `repo-a`, followed by `repo-b` and then `repo-c`. 

6.  **Final Validation:** The test suite in `repo-c` is run a final time. It is now expected to **pass**, demonstrating that `tako` correctly resolved the binary incompatibility by rebuilding the entire dependency chain.
