# RBAC live test report

**Date:** 2026-10-28  
**Environment checked:** [unifai.yespanchi.com](https://unifai.yespanchi.com)  
**Status:** Partial — Admin page access and role-policy state checked; full User/Sub-Admin sign-in and CRUD testing still needs a dedicated test identity and action-time approval for live access changes.

## What was checked

- Opened the production workspace in the existing signed-in Admin session.
- Navigated 52 distinct workspace pages. They loaded without a visible `Forbidden`, unauthorized, or failed-to-load message in the Admin session.
- Opened **Roles & Permissions** and inspected the Admin, User, and Sub-Admin policy targets. The screen reports 59 workspace features across 8 categories. The RBAC page browser console had no errors at the time checked.
- Reviewed the User Governance and account-creation flow. No account was submitted and no live records were changed.

## Findings

1. **Admin default:** The “All Admins” policy is shown as Full Access.
2. **User default:** The “All Users” policy is shown as View Only, including Prompt Repository.
3. **Prompt Repository inconsistency:** The same screen says User defaults have Full Access on Prompt Repository, but the Prompt Repository row under “All Users” shows View Only. This is a verified UI/policy mismatch.
4. **Individual User override:** The existing User account `febi` has an individual Full Access override. That overrides the global User View Only policy and prevents this account from serving as a View Only test until its override is changed or a separate test account is used.
5. **Sub-Admin:** The page shows zero Sub-Admin accounts, and the current “All Sub Admins” policy is No Access across the matrix. The intended “full access only for assigned features” behavior is therefore not validated yet.
6. **Reported 403s:** Your supplied screenshots show Forbidden errors for Local Cache settings and Large Payload configuration. I did not reproduce those errors in the Admin session; a User session is required to verify them.

## Test results

| Test | Result | Notes |
|---|---|---|
| Admin workspace page loading | Pass, 52 pages | Admin context only; does not prove User permissions are correct. |
| Admin sign-in flow | Not tested | The browser was already signed in as Admin. |
| User sign-in and logout | Not tested | Requires the test User session; enter any password or OTP yourself in the browser. |
| Create Admin/User/Sub-Admin accounts | Not tested | The form needs real test email address(es); no accounts were created. |
| User View Only | Not tested as a User | Global policy is View Only, but an existing User has an individual Full Access override. |
| User Create Only / Edit Only / Delete Only / Full Access | Not tested | Each saved permission change affects live access and needs approval immediately before saving. |
| Sub-Admin assigned-feature access | Not tested | No Sub-Admin account exists; current global policy is No Access. |
| Create/Edit/Delete actions across features | Not tested | No production records were changed. |
| Cache / Large Payload 403 reproduction | Not tested as a User | Your screenshots document the errors; the Admin route check cannot validate a User's API permissions. |

## What is needed to finish

- A dedicated test email address for each account you want created. I will not invent an email or create accounts using a real employee's address.
- You to complete password and OTP entry in the browser when a test sign-in is needed.
- Immediate approval before I save the first live permission change. The planned test would temporarily apply View Only, Create Only, Edit Only, Delete Only, then Full Access to a test User, and restore its original policy at the end. This changes security access on the production site.

No live permission changes, account creations, or CRUD operations were performed during this check.

## Screenshot evidence

The following are the original screenshots you supplied. They are included as reported evidence, not as fresh captures from this run.

### Local Cache Forbidden error

![User-provided Local Cache screenshot](screenshots/user-reported-cache-forbidden.png)

### Large Payload Forbidden error

![User-provided Large Payload screenshot](screenshots/user-reported-large-payload-forbidden.png)

