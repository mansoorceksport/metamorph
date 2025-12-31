# Testing Strategy: "Golden Path" Integration Tests

## Problem
Currently, testing the critical user flows (Tenant creation -> Admin assignment -> Coach assignment -> Member flow) is manual ("ClickOps") using Postman and Google Docs. This is:
- Slow (~20 mins per run)
- Prone to human error
- Not repeatable in CI/CD

## Proposed Solution: E2E Go Test Suite

Create a dedicated test suite (e.g., `tests/e2e_flow_test.go`) that programmatically executes the entire lifecycle of a tenant.

### The "Golden Path" Flow
The test will execute the following steps in order, passing data (tokens, IDs) from one step to the next:

1.  **Super Admin Login**
    - `POST /v1/auth/login` (as Super Admin)
    - **Assert**: 200 OK, Token returned.
2.  **Create Tenant**
    - `POST /v1/platform/tenants`
    - **Assert**: 201 Created, `tenant_id` returned.
3.  **Create Branch**
    - `POST /v1/platform/branches`
    - **Assert**: Branch created, linked to Tenant.
4.  **Create Tenant Admin**
    - `POST /v1/platform/tenant-admins`
    - **Assert**: User created with `tenant_admin` role.
5.  **Tenant Admin Login**
    - `POST /v1/auth/login` (as Tenant Admin)
    - **Assert**: Token allows tenant-scoped operations.
6.  **Create Coach**
    - `POST /v1/tenant-admin/coaches`
    - **Assert**: Coach created in correct tenant.
7.  **Create Member**
    - `POST /v1/tenant-admin/users`
    - **Assert**: Member created.
8.  **Assign Coach**
    - `POST /v1/tenant-admin/assignments`
    - **Assert**: Assignment successful.
    - **Negative Test**: Try assigning coach from different tenant (Expect 400/403).
9.  **Verify Data Persistence (Critical)**
    - Log in as Coach.
    - Fetch own profile.
    - **Assert**: `home_branch_id` is NOT empty (validates partial update logic).

### Implementation Details
- **Location**: `tests/` folder in project root.
- **Mocking**: Abstract `FirebaseAuth` interface to allow "mock" logins for test users without hitting real Google servers.
- **Database Strategy: Disposable Docker Containers (Recommended)**
    - Use **[testcontainers-go](https://github.com/testcontainers/testcontainers-go)** to programmatically spin up a fresh MongoDB container for each test run.
    - **Benefits**:
        - **Isolation**: Each test runs against a pristine database. No flaky tests from leftover data.
        - **Safety**: Zero risk of accidentally wiping the local development database (`.env` is ignored).
        - **Zero Setup**: No need to manually create/drop test databases; standard Docker is all that's required.
- **Cleanup**: The container is automatically destroyed when the test finishes.

### Benefits
- **Speed**: Entire flow runs in < 5 seconds.
- **Confidence**: Can run on every `git push`.
- **Documentation**: acts as living proof of how the system works.
