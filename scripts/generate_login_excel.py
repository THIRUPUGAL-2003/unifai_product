import os
import openpyxl
from openpyxl.styles import Font, PatternFill, Alignment, Border, Side
from openpyxl.utils import get_column_letter
from openpyxl.worksheet.datavalidation import DataValidation

wb = openpyxl.Workbook()

# Setup Sheets
ws_setup = wb.active
ws_setup.title = "QA Test Plan & Setup"
ws_setup.views.sheetView[0].showGridLines = True

ws_tests = wb.create_sheet(title="Auth & Lifecycle Test Cases")
ws_tests.views.sheetView[0].showGridLines = True

ws_bugs = wb.create_sheet(title="Defect Log")
ws_bugs.views.sheetView[0].showGridLines = True

# Palettes & Styling
navy_header_fill = PatternFill(start_color="0F172A", end_color="0F172A", fill_type="solid")
slate_fill = PatternFill(start_color="1E293B", end_color="1E293B", fill_type="solid")
light_blue_fill = PatternFill(start_color="F0F9FF", end_color="F0F9FF", fill_type="solid")
section_bg_fill = PatternFill(start_color="334155", end_color="334155", fill_type="solid")
section_plan_fill = PatternFill(start_color="E2E8F0", end_color="E2E8F0", fill_type="solid")

header_font = Font(name="Calibri", size=11, bold=True, color="FFFFFF")
section_title_font = Font(name="Calibri", size=12, bold=True, color="FFFFFF")
title_font = Font(name="Calibri", size=16, bold=True, color="0F172A")
subtitle_font = Font(name="Calibri", size=11, italic=True, color="475569")
bold_font = Font(name="Calibri", size=11, bold=True, color="0F172A")
regular_font = Font(name="Calibri", size=10, color="1E293B")
code_font = Font(name="Consolas", size=9.5, color="0369A1")

thin_border = Border(
    left=Side(style="thin", color="CBD5E1"),
    right=Side(style="thin", color="CBD5E1"),
    top=Side(style="thin", color="CBD5E1"),
    bottom=Side(style="thin", color="CBD5E1")
)

# ==============================================================================
# SHEET 1: QA TEST PLAN & SETUP
# ==============================================================================
ws_setup.column_dimensions["B"].width = 24
ws_setup.column_dimensions["C"].width = 68

ws_setup.row_dimensions[2].height = 32
ws_setup["B2"] = "UnifAI Enterprise Platform — QA Test Execution Plan & Lifecycle Matrix"
ws_setup["B2"].font = title_font
ws_setup["B3"] = "End-to-End Test Plan for Authentication, User Lifecycle (Approve, Reject, Delete), and Account Block/Unblock Flows"
ws_setup["B3"].font = subtitle_font

server_domain = os.getenv("SERVER_DOMAIN", "")
if not server_domain and os.path.isfile(".env"):
    try:
        with open(".env", "r", encoding="utf-8") as f:
            for line in f:
                if line.startswith("SERVER_DOMAIN="):
                    server_domain = line.split("=", 1)[1].strip().strip("'\"")
    except Exception:
        pass
if not server_domain:
    server_domain = "https://<SERVER_DOMAIN>"

setup_sections = [
    ("1. GENERAL PROJECT INFORMATION", [
        ("Project Name", "UnifAI Enterprise AI Control Plane & Browser Guard"),
        ("Module Under Test", "Authentication & User Lifecycle (Login, OTP, Temp Password, Approvals, Rejections, Deletions, Block & Unblock)"),
        ("Document Version", "v2.5 (Complete Enterprise Auth, Approvals & Block Lifecycle Specification)"),
        ("Target Test Date", "October 2026"),
        ("QA Objective", "Validate all 8 authentication & user management sub-models, error handling, and security session lifecycle."),
    ]),
    ("2. TEST ENVIRONMENT & ENDPOINTS", [
        ("Login URL", f"http://localhost:3000/login (or production: {server_domain}/login)"),
        ("Users Management URL", "http://localhost:3000/workspace/users"),
        ("Backend Gateway API", f"http://localhost:6000 (or production: {server_domain})"),
        ("Supported Browsers", "Google Chrome (Latest), Microsoft Edge (Latest), Brave (Latest)"),
        ("Operating Systems", "Windows 11 / Windows 10, macOS, Linux"),
    ]),
    ("3. TEST ACCOUNTS & DATA FOR QA TESTERS", [
        ("Primary Active Account", "Username: thirupugal  |  Email: thirupugals2003@gmail.com"),
        ("Admin / Corporate Account", "Username: Thirupugal (thirupugal)  |  Email: thirupugal@yespanchi.com"),
        ("Temporary Password Account", "User with 'must_change_password=true' flag set in database"),
        ("Pending Sign-up Account", "User registered on /signup awaiting Admin acceptance"),
        ("Blocked Account", "Account with status='disabled' for block enforcement testing"),
        ("Non-Existent Account (Negative)", "Username: non_existent_user_99  |  Email: fake_user_99@test.com"),
    ]),
    ("4. INCLUDED AUTHENTICATION & USER LIFECYCLE MODELS", [
        ("Model 1: Primary Login Form", "Standard sign-in with username/email, password, eye toggle, and validations."),
        ("Model 2: Forgot Password Modal", "Email/username entry to trigger one-time password (OTP) email request."),
        ("Model 3: Reset Password & OTP Modal", "6-digit OTP code verification, resend cooldown timer, and new password setup."),
        ("Model 4: Forgot Username Modal", "Email entry to retrieve registered username via email notification."),
        ("Model 5: Force Change Password Modal", "First-time temporary password replacement with 5 live complexity rules."),
        ("Model 6: Global Navigation & Goto Redirection", "Sign-up links, deep goto redirection post-login, and home return."),
        ("Model 7: User Approvals, Rejections & Deletions", "Admin Accept/Approve, Deny/Reject, Re-accept, and Permanent Delete actions."),
        ("Model 8: Account Block, Unblock & Invalidation", "Admin Block action, instant active session purge, Account Blocked alert, and Unblock."),
    ]),
    ("5. QA SIGN-OFF BLOCK", [
        ("QA Lead Name", ""),
        ("Execution Date", ""),
        ("Overall Decision", "PASS / CONDITIONAL PASS / REJECTED"),
        ("Sign-Off Remarks", ""),
    ])
]

s_row = 5
for section_title, rows in setup_sections:
    ws_setup.cell(row=s_row, column=2, value=section_title).font = bold_font
    ws_setup.cell(row=s_row, column=2).fill = section_plan_fill
    ws_setup.cell(row=s_row, column=3).fill = section_plan_fill
    ws_setup.row_dimensions[s_row].height = 24
    s_row += 1
    
    for label, val in rows:
        c1 = ws_setup.cell(row=s_row, column=2, value=label)
        c1.font = bold_font
        c1.border = thin_border
        c1.fill = light_blue_fill
        
        c2 = ws_setup.cell(row=s_row, column=3, value=val)
        c2.font = regular_font
        c2.border = thin_border
        ws_setup.row_dimensions[s_row].height = 22
        s_row += 1
    s_row += 1


# ==============================================================================
# SHEET 2: LOGIN & ALL MODALS TEST CASES
# ==============================================================================
test_headers = [
    "Test Case ID",
    "Model / Section",
    "Test Scenario / Objective",
    "Preconditions",
    "Step-by-Step Test Procedure",
    "Test Data / Inputs",
    "Expected Result (Pass Criteria)",
    "Actual Result (Tester Notes)",
    "Status",
    "Priority",
    "Defect / Bug ID",
    "Tester Name",
    "Execution Date"
]

# Sections with test cases
sections_data = [
    # --------------------------------------------------------------------------
    # SECTION 1: PRIMARY LOGIN FORM MODEL
    # --------------------------------------------------------------------------
    {
        "section_name": "SECTION 1: PRIMARY LOGIN FORM (SIGN IN MODEL)",
        "cases": [
            [
                "TC_LOGIN_01",
                "Primary Login",
                "Verify submission behavior when both Username and Password are left blank.",
                "Login page is displayed (http://localhost:3000/login)",
                "1. Leave Username field empty.\n2. Leave Password field empty.\n3. Click 'Sign In' button.",
                "Username: [Blank]\nPassword: [Blank]",
                "• Form does not submit.\n• Red inline message 'Please fill in this field' appears below Username.\n• Red inline message 'Please fill in this field' appears below Password.\n• Top alert banner displays: 'Please fill in all required fields'.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_LOGIN_02",
                "Primary Login",
                "Verify validation when Username is empty and Password is provided.",
                "Login page is displayed.",
                "1. Leave Username field empty.\n2. Enter valid password in Password input.\n3. Click 'Sign In' button.",
                "Username: [Blank]\nPassword: ValidPassword123",
                "• Form does not submit.\n• Username input displays inline error 'Please fill in this field'.\n• Password input displays no error.\n• Top banner displays 'Please fill in all required fields'.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_LOGIN_03",
                "Primary Login",
                "Verify validation when Username is provided and Password is left empty.",
                "Login page is displayed.",
                "1. Enter username.\n2. Leave Password field empty.\n3. Click 'Sign In' button.",
                "Username: thirupugal\nPassword: [Blank]",
                "• Form does not submit.\n• Password input displays inline error 'Please fill in this field'.\n• Username input displays no error.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_LOGIN_04",
                "Primary Login",
                "Verify inline error clears immediately when user starts typing.",
                "Validation error is visible under Username.",
                "1. Focus cursor into Username field.\n2. Type any character (e.g. 't').",
                "Keypress: 't'",
                "• The red inline validation error under Username immediately disappears upon first keystroke.",
                "", "Untested", "P2 - Medium", "", "", ""
            ],
            [
                "TC_LOGIN_05",
                "Primary Login",
                "Verify successful authentication with valid Username and Password (Happy Path).",
                "Active user account exists.",
                "1. Enter valid username.\n2. Enter correct password.\n3. Click 'Sign In' button.",
                "Username: thirupugal\nPassword: [Correct Password]",
                "• Sign In button displays loading spinner.\n• User is authenticated successfully.\n• Page redirects to dashboard (/workspace).\n• Auth session token is saved in browser.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_LOGIN_06",
                "Primary Login",
                "Verify successful authentication using registered Email address as username.",
                "Active user account exists with registered email.",
                "1. Enter registered email address into Username box.\n2. Enter correct password.\n3. Click 'Sign In'.",
                "Username: thirupugals2003@gmail.com\nPassword: [Correct Password]",
                "• System accepts email format as identifier.\n• Successfully logs in and redirects to workspace.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_LOGIN_07",
                "Primary Login",
                "Verify error message on submitting wrong password for valid user.",
                "Valid user exists in system.",
                "1. Enter valid username.\n2. Enter incorrect password.\n3. Click 'Sign In'.",
                "Username: thirupugal\nPassword: WrongPassword@999",
                "• Login is rejected.\n• User remains on login screen.\n• Error banner displays: 'Invalid username or password'.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_LOGIN_08",
                "Primary Login",
                "Verify error message on submitting non-existent username.",
                "No account exists with this name.",
                "1. Enter non-existent username.\n2. Enter any password.\n3. Click 'Sign In'.",
                "Username: non_existent_user_99\nPassword: AnyPassword123",
                "• Login fails.\n• Error banner displays: 'Invalid username or password'.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_LOGIN_09",
                "Primary Login",
                "Verify Password Eye icon toggle reveals and conceals password characters.",
                "User is on login view.",
                "1. Type password in Password field (characters show as bullets).\n2. Click Eye icon on right side of input.\n3. Click Eye-off icon again.",
                "Password: SecretPassword2026",
                "• Initially masked (••••••••).\n• Clicking eye reveals password in plain text.\n• Clicking eye-off conceals password back into bullets.",
                "", "Untested", "P2 - Medium", "", "", ""
            ],
            [
                "TC_LOGIN_10",
                "Primary Login",
                "Verify pressing 'Enter' key inside Password field submits the login form.",
                "Credentials entered in fields.",
                "1. Enter valid username.\n2. Enter valid password.\n3. Press Enter key on keyboard.",
                "Keypress: [Enter]",
                "• Form submits immediately without needing mouse click.",
                "", "Untested", "P2 - Medium", "", "", ""
            ]
        ]
    },

    # --------------------------------------------------------------------------
    # SECTION 2: FORGOT PASSWORD MODAL MODEL
    # --------------------------------------------------------------------------
    {
        "section_name": "SECTION 2: FORGOT PASSWORD MODAL (OTP REQUEST MODEL)",
        "cases": [
            [
                "TC_FORGOT_01",
                "Forgot Password Modal",
                "Verify 'Forgot password?' link transitions view to Forgot Password modal.",
                "User is on Primary Login view.",
                "1. Click the 'Forgot password?' link above Password field.",
                "Click: 'Forgot password?'",
                "• View transitions smoothly to Forgot Password screen.\n• Header title updates to 'Forgot Password'.\n• Displays description, Email/Username input, and 'Send verification code' button.\n• 'Back to Sign In' arrow link is visible.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_FORGOT_02",
                "Forgot Password Modal",
                "Verify validation error when clicking 'Send verification code' with empty input.",
                "User is on Forgot Password modal.",
                "1. Leave Email / Username input completely empty.\n2. Click 'Send verification code' button.",
                "Input: [Blank]",
                "• Submission is blocked.\n• Red inline message 'Please fill in this field' appears below input.\n• Top alert banner displays: 'Please fill in your email address or username'.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_FORGOT_03",
                "Forgot Password Modal",
                "Verify submitting valid email triggers OTP and automatically switches to Reset Password modal.",
                "Valid registered account exists.",
                "1. Enter valid registered email address.\n2. Click 'Send verification code' button.",
                "Email: thirupugals2003@gmail.com",
                "• Button shows 'Sending verification code...' spinner.\n• API generates OTP code.\n• Info banner displays: 'If an account matches, a one-time code was sent by email'.\n• Automatically navigates to the 'Reset Password' OTP verification modal.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_FORGOT_04",
                "Forgot Password Modal",
                "Verify 'Back to Sign In' button returns cleanly to primary login view.",
                "User is on Forgot Password modal.",
                "1. Click '< Back to Sign In' button at top of card.",
                "Click: 'Back to Sign In'",
                "• View switches back to primary Login form.\n• All inputs and errors are cleared.",
                "", "Untested", "P2 - Medium", "", "", ""
            ]
        ]
    },

    # --------------------------------------------------------------------------
    # SECTION 3: RESET PASSWORD & OTP VERIFICATION MODAL MODEL
    # --------------------------------------------------------------------------
    {
        "section_name": "SECTION 3: RESET PASSWORD & OTP VERIFICATION MODAL MODEL",
        "cases": [
            [
                "TC_RESET_01",
                "Reset Password Modal",
                "Verify validation when clicking 'Verify Code' with empty OTP input.",
                "User is on Reset Password modal.",
                "1. Leave OTP code field empty.\n2. Click 'Verify Code' button.",
                "OTP: [Blank]",
                "• Red inline error 'Please fill in this field' appears under OTP input.\n• Alert banner displays: 'Please enter the 6-digit OTP code'.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_RESET_02",
                "Reset Password Modal",
                "Verify validation when entering fewer than 6 digits for OTP.",
                "User is on Reset Password modal.",
                "1. Enter 4 digits (e.g. '1234') in OTP field.\n2. Click 'Verify Code'.",
                "OTP: '1234'",
                "• Red inline error displays: 'OTP must be 6 digits'.\n• Alert banner displays: 'OTP must be 6 digits'.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_RESET_03",
                "Reset Password Modal",
                "Verify rejection on entering an invalid / wrong 6-digit OTP code.",
                "User is on Reset Password modal.",
                "1. Enter incorrect 6-digit code (e.g. '999999').\n2. Click 'Verify Code'.",
                "OTP: '999999'",
                "• Button shows 'Verifying...' spinner.\n• Verification fails.\n• Error banner displays: 'Invalid or expired OTP code'.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_RESET_04",
                "Reset Password Modal",
                "Verify successful OTP verification unlocks new password fields.",
                "Valid OTP received by email.",
                "1. Enter correct 6-digit OTP code.\n2. Click 'Verify Code'.",
                "OTP: [Valid 6-digit Code]",
                "• Verification succeeds.\n• Success banner displays: 'OTP verified successfully! Please enter your new password below.'.\n• New Password and Confirm Password inputs are enabled for entry.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_RESET_05",
                "Reset Password Modal",
                "Verify Resend OTP countdown timer and cooldown enforcement.",
                "User just requested OTP.",
                "1. Check the 'Resend code' button.\n2. Observe countdown timer (120s cooldown e.g. 'Resend in 119s').\n3. Attempt to click resend while timer is running.",
                "Timer: 120s countdown",
                "• Resend button is disabled during countdown.\n• Countdown decrements by 1 second every second.\n• When timer reaches 0, button activates to 'Resend code'.",
                "", "Untested", "P2 - Medium", "", "", ""
            ],
            [
                "TC_RESET_06",
                "Reset Password Modal",
                "Verify live 5-rule Password Strength indicator cards.",
                "OTP is verified (TC_RESET_04).",
                "1. Type characters into New Password field.\n2. Observe the 5 rule indicator cards:\n   - 8+ characters\n   - Uppercase letter\n   - Lowercase letter\n   - Number\n   - Symbol",
                "Typed: 'Password@2026'",
                "• Each rule card turns from grey to vibrant green with a checkmark (✓) as its requirement is fulfilled.\n• If any requirement is missing, its card remains grey/unfulfilled.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_RESET_07",
                "Reset Password Modal",
                "Verify validation error when New Password and Confirm Password do not match.",
                "New Password meets all 5 requirements.",
                "1. In New Password: enter 'Password@2026'.\n2. In Confirm Password: enter 'DifferentPassword@2026'.\n3. Click 'Update Password'.",
                "New: Password@2026\nConfirm: DifferentPassword@2026",
                "• Submission is blocked.\n• Confirm Password field displays inline error: 'Passwords do not match'.\n• Alert banner displays: 'Passwords do not match'.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_RESET_08",
                "Reset Password Modal",
                "Verify successful password reset transitions user back to Login view.",
                "OTP verified and matching new password entered.",
                "1. Enter matching new password meeting all 5 requirements.\n2. Click 'Update Password' button.",
                "New: NewSecurePassword#2026\nConfirm: NewSecurePassword#2026",
                "• Password updates successfully.\n• Success banner displays: 'Password updated. Sign in with your new password.'.\n• Screen automatically returns to primary Login view, ready for new login.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_RESET_09",
                "Reset Password Modal",
                "Verify 'Back to forgot password' returns to previous modal.",
                "User is on Reset Password modal.",
                "1. Click '< Back to forgot password' link at top of card.",
                "Click: 'Back to forgot password'",
                "• Screen transitions back to Forgot Password email entry modal.",
                "", "Untested", "P2 - Medium", "", "", ""
            ]
        ]
    },

    # --------------------------------------------------------------------------
    # SECTION 4: FORGOT USERNAME RECOVERY MODAL MODEL
    # --------------------------------------------------------------------------
    {
        "section_name": "SECTION 4: FORGOT USERNAME RECOVERY MODAL MODEL",
        "cases": [
            [
                "TC_USER_01",
                "Forgot Username Modal",
                "Verify 'Forgot Username?' link transitions to Username recovery modal.",
                "User is on Primary Login view.",
                "1. Click 'Forgot Username?' link at bottom of form.",
                "Click: 'Forgot Username?'",
                "• View switches to 'Forgot Username' modal.\n• Header title updates to 'Forgot Username'.\n• Description explains: 'Enter your registered email address to retrieve your username.'.\n• Email input and 'Retrieve Username' button are displayed.",
                "", "Untested", "P2 - Medium", "", "", ""
            ],
            [
                "TC_USER_02",
                "Forgot Username Modal",
                "Verify validation error when clicking 'Retrieve Username' with blank email.",
                "User is on Forgot Username modal.",
                "1. Leave Email input empty.\n2. Click 'Retrieve Username' button.",
                "Email: [Blank]",
                "• Submission is blocked.\n• Red inline message 'Please fill in this field' appears below Email input.\n• Alert banner displays: 'Please fill in your email address'.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_USER_03",
                "Forgot Username Modal",
                "Verify submitting valid email triggers username recovery notification.",
                "Valid user exists with registered email.",
                "1. Enter registered email address.\n2. Click 'Retrieve Username' button.",
                "Email: thirupugals2003@gmail.com",
                "• Button shows 'Retrieving username...' spinner.\n• Success info banner displays: 'If an account matches, your username was sent to your email.'.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_USER_04",
                "Forgot Username Modal",
                "Verify 'Back to Sign In' returns to primary login view.",
                "User is on Forgot Username modal.",
                "1. Click '< Back to Sign In' button.",
                "Click: 'Back to Sign In'",
                "• View returns cleanly to primary Login form.",
                "", "Untested", "P2 - Medium", "", "", ""
            ]
        ]
    },

    # --------------------------------------------------------------------------
    # SECTION 5: FORCE CHANGE TEMPORARY PASSWORD MODAL MODEL
    # --------------------------------------------------------------------------
    {
        "section_name": "SECTION 5: FORCE CHANGE TEMPORARY PASSWORD MODAL MODEL",
        "cases": [
            [
                "TC_TEMP_01",
                "Temp Password Modal",
                "Verify user with temporary password is automatically redirected to 'Set New Password' modal.",
                "User account has 'must_change_password=true' in database.",
                "1. Enter username and temporary password on Login screen.\n2. Click 'Sign In'.",
                "Username: [Temp User]\nPassword: [Temp Password]",
                "• System verifies temporary credentials.\n• Instead of opening dashboard, user is routed to 'Set New Password' modal.\n• Header displays: 'Set New Password'.\n• Info banner displays: 'Temporary password verified. Please set your new permanent password below.'.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_TEMP_02",
                "Temp Password Modal",
                "Verify validation when submitting empty new password fields.",
                "User is on 'Set New Password' modal.",
                "1. Leave New Password empty.\n2. Leave Confirm Password empty.\n3. Click 'Update Password'.",
                "New: [Blank]\nConfirm: [Blank]",
                "• Form does not submit.\n• Inline error 'Please fill in this field' appears on both fields.\n• Top alert banner displays: 'Please fill in all required fields'.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_TEMP_03",
                "Temp Password Modal",
                "Verify 5 password strength rules must all be satisfied before submission.",
                "User enters weak password.",
                "1. In New Password, enter 'abc' (too short, no upper, no number, no symbol).\n2. Attempt to submit.",
                "New: 'abc'",
                "• Submission blocked.\n• Inline error displays: 'Must meet all 5 requirements'.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_TEMP_04",
                "Temp Password Modal",
                "Verify validation when Confirm Password does not match New Password.",
                "New Password satisfies all 5 rules.",
                "1. Enter valid New Password.\n2. Enter different password in Confirm Password.\n3. Click 'Update Password'.",
                "New: PermanentPass#2026\nConfirm: MismatchPass#2026",
                "• Submission blocked.\n• Confirm Password field highlights with: 'Passwords do not match'.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_TEMP_05",
                "Temp Password Modal",
                "Verify successful permanent password update routes user directly to workspace.",
                "Valid matching passwords entered.",
                "1. Enter matching new permanent password.\n2. Click 'Update Password'.",
                "New: PermanentPass#2026\nConfirm: PermanentPass#2026",
                "• Button shows loading spinner.\n• Success message: 'Password updated successfully! Entering dashboard...'.\n• User is automatically redirected to workspace dashboard (/workspace).",
                "", "Untested", "P1 - High", "", "", ""
            ]
        ]
    },

    # --------------------------------------------------------------------------
    # SECTION 6: GLOBAL NAVIGATION & POST-LOGIN REDIRECTION
    # --------------------------------------------------------------------------
    {
        "section_name": "SECTION 6: GLOBAL NAVIGATION & REDIRECTION",
        "cases": [
            [
                "TC_NAV_01",
                "Global Navigation",
                "Verify 'Sign up' link in top header and bottom form navigates to /signup.",
                "User is on login view.",
                "1. Click 'Sign Up' button in top header.\n2. Click 'Sign up' link at bottom.",
                "Click: 'Sign Up'",
                "• Browser smoothly navigates to /signup registration page.",
                "", "Untested", "P2 - Medium", "", "", ""
            ],
            [
                "TC_NAV_02",
                "Global Navigation",
                "Verify top header 'Back to Home' arrow button navigates to landing page (/).",
                "User is on login view.",
                "1. Click the top-left '< Back to Home' button.",
                "Click: 'Back to Home'",
                "• Browser navigates back to root home landing page (http://localhost:3000/).",
                "", "Untested", "P3 - Low", "", "", ""
            ],
            [
                "TC_NAV_03",
                "Deep Redirection",
                "Verify post-login redirection respects the 'goto' URL parameter.",
                "User accesses protected deep URL: /login?goto=/workspace/browser-ai",
                "1. In browser, navigate to: http://localhost:3000/login?goto=/workspace/browser-ai\n2. Enter valid login credentials.\n3. Click 'Sign In'.",
                "URL: /login?goto=/workspace/browser-ai\nUsername: thirupugal\nPassword: [Correct Password]",
                "• After authentication, system redirects directly to /workspace/browser-ai instead of default /workspace.",
                "", "Untested", "P1 - High", "", "", ""
            ]
        ]
    },

    # --------------------------------------------------------------------------
    # SECTION 7: REGISTRATION APPROVAL, REJECTION & DELETION LIFECYCLE
    # --------------------------------------------------------------------------
    {
        "section_name": "SECTION 7: USER REGISTRATION APPROVAL, REJECTION & DELETION",
        "cases": [
            [
                "TC_APPR_01",
                "User Approvals",
                "Verify Admin can accept/approve a pending user registration.",
                "Admin is logged in. A pending user registration exists in Users > Pending Approvals.",
                "1. Navigate to /workspace/users.\n2. Filter or switch to 'Pending Approvals' view.\n3. Locate the pending user row.\n4. Click 'Accept' (green check icon/button).\n5. Observe UI response and toast notifications.",
                "Target: Pending User row\nAction: Click 'Accept'",
                "• Target row shows loading state during submission.\n• Green toast appears: '<username> approved — notification email sent'.\n• User is removed from Pending Approvals and moved to Active Users list.\n• Approval email containing direct login URL is dispatched to the user.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_APPR_02",
                "Approved Login",
                "Verify user can successfully sign in after Admin approval.",
                "User registration was previously pending and just approved by Admin.",
                "1. Navigate to /login.\n2. Enter registered Username/Email.\n3. Enter registered Password.\n4. Click 'Sign In'.",
                "Username: [Approved Username]\nPassword: [Registered Password]",
                "• Credentials are authenticated successfully.\n• User session is established.\n• User is smoothly redirected to the workspace dashboard.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_APPR_03",
                "Pending Login Block",
                "Verify login attempt is prevented while user account is pending Admin approval.",
                "User has completed self-registration but Admin has not approved yet.",
                "1. Navigate to /login.\n2. Enter registered Username and Password.\n3. Click 'Sign In'.",
                "Username: [Pending User]\nPassword: [User Password]",
                "• Authentication is blocked (HTTP 403 / Conflict).\n• Error banner displays: 'Your account is pending administrator approval' or equivalent.\n• User cannot access workspace.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_REJ_01",
                "User Rejection",
                "Verify Admin can deny/reject a pending user registration.",
                "Admin is in /workspace/users with pending users listed.",
                "1. Locate pending user row in Users view.\n2. Click 'Deny' / 'Reject' button.\n3. Observe UI response and status changes.",
                "Target: Pending User row\nAction: Click 'Deny'",
                "• Target row processes request.\n• Toast notification displays: '<username> denied — notification email sent'.\n• User is moved from Pending to 'Rejected' tab with status 'rejected'.\n• Rejection notification email is dispatched to user's registered email.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_REJ_02",
                "Rejected Login Block",
                "Verify login attempt is prevented when an account has been rejected.",
                "Account was rejected by Admin.",
                "1. Navigate to /login.\n2. Enter credentials for the rejected account.\n3. Click 'Sign In'.",
                "Username: [Rejected User]\nPassword: [User Password]",
                "• Sign in is blocked.\n• Error banner displays notice that registration was denied/rejected.\n• Workspace access is denied.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_REJ_03",
                "Re-accept Rejected User",
                "Verify Admin can re-accept a previously rejected user from Rejected view.",
                "Admin navigates to /workspace/users with 'rejected' role/status filter active.",
                "1. Switch to 'Rejected' accounts view.\n2. Locate the rejected user row.\n3. Click the green 'Accept' button.\n4. Observe status update and notification.",
                "Target: Rejected User row\nAction: Click 'Accept' (Re-accept)",
                "• Action button shows loading state.\n• Toast displays: '<username> approved — notification email sent'.\n• User is removed from Rejected list and reinstated into Active Users list.\n• Approval email with login link is re-sent to user's email.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_REJ_04",
                "Re-accepted User Login",
                "Verify previously rejected user can log in immediately after being re-accepted.",
                "Rejected user was re-accepted by Admin in TC_REJ_03.",
                "1. Open /login in browser.\n2. Enter re-accepted user credentials.\n3. Click 'Sign In'.",
                "Username: [Re-accepted User]\nPassword: [User Password]",
                "• Login succeeds without any blocks or errors.\n• User is successfully routed into workspace dashboard.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_DEL_01",
                "Delete Rejected User",
                "Verify Admin can permanently delete a rejected user account.",
                "Admin is in 'Rejected' users view.",
                "1. In Rejected users list, click 'Delete' (red trash button) on a user.\n2. In the confirmation dialog, review warning details.\n3. Click 'Delete User' confirm button.",
                "Target: Rejected User\nAction: Confirm Permanent Deletion",
                "• Confirmation modal displays with destructive action styling.\n• On confirm, user record is permanently removed from the database.\n• Toast displays: 'User deleted successfully'.\n• User row disappears from the table.",
                "", "Untested", "P2 - Medium", "", "", ""
            ],
            [
                "TC_DEL_02",
                "Delete Active User",
                "Verify Admin can permanently delete an active user account from Active Users list.",
                "Admin is in /workspace/users active list.",
                "1. Locate user in Active Users table.\n2. Click the trash icon / delete action.\n3. Confirm deletion in the popup dialog.",
                "Target: Active User\nAction: Confirm Delete",
                "• Modal warns: 'Are you sure you want to delete this user? This action cannot be undone.'.\n• On submit, record is removed from database.\n• Any active sessions for that user are immediately invalidated.\n• Toast displays: 'User deleted successfully'.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_DEL_03",
                "Deleted User Login Check",
                "Verify login is completely blocked after permanent account deletion.",
                "Account was deleted in TC_DEL_01 or TC_DEL_02.",
                "1. Navigate to /login.\n2. Attempt to sign in with credentials of the deleted account.\n3. Click 'Sign In'.",
                "Username: [Deleted Username]\nPassword: [Old Password]",
                "• Login is rejected with error: 'Invalid username or password' or 'User not found'.\n• System does not recognize deleted credentials.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_DEL_04",
                "RBAC Guard on Deletion",
                "Verify non-admin / unauthorized roles cannot delete user accounts.",
                "Logged in as a standard user or sub-admin without delete privileges.",
                "1. Check if Users management is accessible or if delete buttons are disabled.\n2. Attempt direct DELETE /api/session/users/{id} call.",
                "Role: Standard User / Sub-Admin without Delete scope",
                "• Delete button is hidden/disabled in UI.\n• Direct API call returns HTTP 403 Forbidden.",
                "", "Untested", "P2 - Medium", "", "", ""
            ]
        ]
    },

    # --------------------------------------------------------------------------
    # SECTION 8: ACCOUNT BLOCK, UNBLOCK & SECURITY TERMINATION LIFECYCLE
    # --------------------------------------------------------------------------
    {
        "section_name": "SECTION 8: ACCOUNT BLOCK, UNBLOCK & SECURITY TERMINATION",
        "cases": [
            [
                "TC_BLOCK_01",
                "Block User Action",
                "Verify Admin can block an active user account with instant session termination.",
                "Admin is in /workspace/users active users view. Target user is active.",
                "1. Locate active user row.\n2. Click the 'Block' (Ban icon) button.\n3. Observe UI updates and network call to /api/session/users/{id}/block.",
                "Target: Active User\nAction: Click 'Block'",
                "• User status is updated to 'disabled' in database.\n• All active sessions/tokens for this user are revoked immediately.\n• Toast displays: '<username> blocked — security notification email sent'.\n• Security notice email is dispatched to user's inbox.\n• User is moved to 'Blocked Accounts' tab.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_BLOCK_02",
                "Blocked User Login Attempt",
                "Verify blocked user cannot sign in and sees dedicated 'Account Blocked' alert banner.",
                "User account is in 'disabled' (blocked) status.",
                "1. Open /login in browser.\n2. Enter blocked user credentials.\n3. Click 'Sign In'.",
                "Username: [Blocked User]\nPassword: [Valid Password]",
                "• Sign-in is strictly blocked.\n• Dedicated red alert banner appears with Ban icon and header: 'Account Blocked'.\n• Descriptive message informs user: 'Security Notice: Your Gateway account has been blocked by an administrator. You will not be able to sign in...'.\n• User is NOT granted access to workspace.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_BLOCK_03",
                "Active Session Invalidation",
                "Verify active user session is terminated immediately when account is blocked.",
                "Target user is currently logged into dashboard in Browser Window B. Admin blocks user in Window A.",
                "1. In Window A, Admin clicks 'Block' on target user.\n2. In Window B, target user attempts to navigate or perform any API action.",
                "Action: User clicks any link or triggers API in Window B",
                "• API returns HTTP 401 Unauthorized because server sessions were purged.\n• User is automatically logged out and redirected to /login.\n• Further access is prevented.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_BLOCK_04",
                "Blocked Accounts Tab View",
                "Verify Blocked Accounts tab displays all blocked accounts with correct metadata.",
                "One or more accounts are blocked.",
                "1. Navigate to /workspace/users.\n2. Select 'Blocked' role filter / tab.\n3. Inspect table columns and action buttons.",
                "Filter: 'Blocked' accounts",
                "• Displays total count of blocked accounts.\n• Table shows ID, Username with 'Blocked' badge, Email, Role badge, and Updated At timestamp.\n• Actions column displays 'Unblock' (green check) and 'Delete' (red trash) buttons.",
                "", "Untested", "P2 - Medium", "", "", ""
            ],
            [
                "TC_BLOCK_05",
                "Unblock User Action",
                "Verify Admin can unblock an account and restore user access.",
                "Admin is in 'Blocked Accounts' view.",
                "1. Locate blocked user row in Blocked Accounts tab.\n2. Click 'Unblock' button.\n3. Observe UI response and toast notifications.",
                "Target: Blocked User\nAction: Click 'Unblock'",
                "• Status switches to 'approved' in database.\n• Toast displays: '<username> unblocked — notification email sent'.\n• Unblock notification email containing sign-in link is dispatched to user.\n• User is removed from Blocked tab and restored to Active Users list.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_BLOCK_06",
                "Unblocked User Login",
                "Verify unblocked user can sign in normally with existing credentials.",
                "User was unblocked in TC_BLOCK_05.",
                "1. Navigate to /login.\n2. Enter Username and Password.\n3. Click 'Sign In'.",
                "Username: [Unblocked User]\nPassword: [Existing Password]",
                "• Login is authenticated smoothly without 'Account Blocked' error.\n• User is successfully directed to workspace dashboard.",
                "", "Untested", "P1 - High", "", "", ""
            ],
            [
                "TC_BLOCK_07",
                "Delete Blocked User",
                "Verify Admin can permanently delete an account directly from Blocked Accounts tab.",
                "Admin is in Blocked Accounts view.",
                "1. Click 'Delete' button on a blocked user row.\n2. Confirm deletion in the confirmation modal.\n3. Verify table updates.",
                "Target: Blocked User\nAction: Confirm Permanent Deletion",
                "• Modal warns of permanent deletion.\n• On confirm, user record is purged from database.\n• Toast displays: 'User deleted successfully'.\n• Row is removed from Blocked table.",
                "", "Untested", "P2 - Medium", "", "", ""
            ],
            [
                "TC_BLOCK_08",
                "Admin Self/Peer Block Guard",
                "Verify system prevents blocking administrator accounts.",
                "Logged in as Admin. Viewing users list containing Admin accounts.",
                "1. Observe 'Block' button on rows with Role = Admin.\n2. Attempt to trigger block API on an Admin user ID.",
                "Target: Admin user row",
                "• 'Block' button is disabled for Administrator accounts.\n• Backend API returns HTTP 403 Forbidden with: 'Cannot block an administrator account'.",
                "", "Untested", "P2 - Medium", "", "", ""
            ]
        ]
    }
]

# Write Table Headers
ws_tests.append(test_headers)
for col_num in range(1, len(test_headers) + 1):
    cell = ws_tests.cell(row=1, column=col_num)
    cell.fill = navy_header_fill
    cell.font = header_font
    cell.alignment = Alignment(horizontal="center", vertical="center", wrap_text=True)
    cell.border = thin_border
ws_tests.row_dimensions[1].height = 28

cur_row = 2

# Populate Sections with visual space and section banners
for section in sections_data:
    # 1. Visual small blank spacing row before each section
    if cur_row > 2:
        ws_tests.row_dimensions[cur_row].height = 12
        cur_row += 1

    # 2. Section Header Banner Row
    ws_tests.merge_cells(start_row=cur_row, start_column=1, end_row=cur_row, end_column=len(test_headers))
    banner_cell = ws_tests.cell(row=cur_row, column=1, value=section["section_name"])
    banner_cell.fill = section_bg_fill
    banner_cell.font = section_title_font
    banner_cell.alignment = Alignment(horizontal="left", vertical="center", indent=1)
    ws_tests.row_dimensions[cur_row].height = 26
    
    for c in range(1, len(test_headers) + 1):
        ws_tests.cell(row=cur_row, column=c).border = thin_border
        ws_tests.cell(row=cur_row, column=c).fill = section_bg_fill
        
    cur_row += 1

    # 3. Test Cases in this section
    for tc in section["cases"]:
        ws_tests.append(tc)
        ws_tests.row_dimensions[cur_row].height = 54
        
        for c_idx in range(1, len(tc) + 1):
            cell = ws_tests.cell(row=cur_row, column=c_idx)
            cell.font = regular_font
            cell.border = thin_border
            
            # Alignments
            if c_idx in (1, 9, 10, 11, 12, 13):
                cell.alignment = Alignment(horizontal="center", vertical="center")
                if c_idx == 1:
                    cell.font = code_font
            elif c_idx in (4, 5, 6, 7, 8):
                cell.alignment = Alignment(horizontal="left", vertical="center", wrap_text=True)
            else:
                cell.alignment = Alignment(horizontal="left", vertical="center")
                
            # Priority color coding
            if c_idx == 10:
                val = str(cell.value)
                if "P1" in val:
                    cell.fill = PatternFill(start_color="FEE2E2", end_color="FEE2E2", fill_type="solid")
                    cell.font = Font(name="Calibri", size=10, bold=True, color="991B1B")
                elif "P2" in val:
                    cell.fill = PatternFill(start_color="FEF3C7", end_color="FEF3C7", fill_type="solid")
                    cell.font = Font(name="Calibri", size=10, bold=True, color="92400E")
                elif "P3" in val:
                    cell.fill = PatternFill(start_color="E0F2FE", end_color="E0F2FE", fill_type="solid")
                    cell.font = Font(name="Calibri", size=10, color="0369A1")
                    
            # Status column default
            if c_idx == 9:
                cell.fill = PatternFill(start_color="F1F5F9", end_color="F1F5F9", fill_type="solid")
                cell.font = Font(name="Calibri", size=10, bold=True, color="64748B")
                
        cur_row += 1

# Add Excel Status Dropdown Validation across all rows
dv_status = DataValidation(type="list", formula1='"Pass,Fail,Blocked,Untested"', allow_blank=True)
ws_tests.add_data_validation(dv_status)
dv_status.add(f"I2:I{cur_row}")

# Column widths for Test Cases
test_col_widths = {
    1: 15,  # Test ID
    2: 24,  # Model / Section
    3: 36,  # Scenario
    4: 26,  # Preconditions
    5: 38,  # Steps
    6: 28,  # Test Data
    7: 42,  # Expected Result
    8: 26,  # Actual Result
    9: 14,  # Status
    10: 14, # Priority
    11: 16, # Defect ID
    12: 16, # Tester Name
    13: 16  # Execution Date
}
for col_num, width in test_col_widths.items():
    ws_tests.column_dimensions[get_column_letter(col_num)].width = width


# ==============================================================================
# SHEET 3: DEFECT LOG
# ==============================================================================
bug_headers = [
    "Defect ID",
    "Linked Test Case ID",
    "Model / Section",
    "Defect Summary / Title",
    "Severity",
    "Steps to Reproduce",
    "Observed Behavior (Bug)",
    "Expected Behavior",
    "Reported By",
    "Date Logged",
    "Status"
]
ws_bugs.append(bug_headers)
for col_num in range(1, len(bug_headers) + 1):
    cell = ws_bugs.cell(row=1, column=col_num)
    cell.fill = slate_fill
    cell.font = header_font
    cell.alignment = Alignment(horizontal="center", vertical="center")
    cell.border = thin_border
ws_bugs.row_dimensions[1].height = 28

for b_row in range(2, 14):
    ws_bugs.row_dimensions[b_row].height = 32
    for b_col in range(1, len(bug_headers) + 1):
        cell = ws_bugs.cell(row=b_row, column=b_col)
        cell.font = regular_font
        cell.border = thin_border
        if b_col in (1, 2, 5, 9, 10, 11):
            cell.alignment = Alignment(horizontal="center", vertical="center")
        else:
            cell.alignment = Alignment(horizontal="left", vertical="center")

bug_col_widths = {
    1: 14, 2: 18, 3: 22, 4: 36, 5: 14, 6: 38, 7: 34, 8: 34, 9: 16, 10: 16, 11: 14
}
for col_num, width in bug_col_widths.items():
    ws_bugs.column_dimensions[get_column_letter(col_num)].width = width


# Save to Desktop and Workspace
out_paths = [
    os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "Auth_and_User_Lifecycle_Test_Cases.xlsx"),
    os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "Login_Test_Cases_Updated.xlsx"),
    os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "Login_Test_Cases.xlsx"),
    os.path.expanduser(r"~\Desktop\Auth_and_User_Lifecycle_Test_Cases.xlsx"),
    os.path.expanduser(r"~\Desktop\Login_Test_Cases.xlsx")
]

saved_files = []
for p in out_paths:
    try:
        os.makedirs(os.path.dirname(p), exist_ok=True)
        wb.save(p)
        saved_files.append(p)
        print(f"Saved: {p.encode('ascii', 'replace').decode()}")
    except Exception as e:
        print(f"Skipped {p.encode('ascii', 'replace').decode()}: {str(e).encode('ascii', 'replace').decode()}")

print("Complete All-Modals & User Lifecycle QA Test Case Workbook generated successfully.")
