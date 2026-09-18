// Package youtube implements YouTube OAuth2 authorization, resumable video upload, and caption synchronization.
package youtube

import (
	"fmt"
	"io"
)

const setupGuideText = `================================================================================
                    TALK_CUT YOUTUBE SETUP GUIDE
================================================================================

This guide walks you through configuring YouTube upload authorization for
talk_cut using Google Cloud OAuth 2.0.

--------------------------------------------------------------------------------
1. OVERVIEW & REQUIREMENTS
--------------------------------------------------------------------------------
- Why OAuth 2.0 Desktop Application?
  YouTube Data API v3 requires 3-legged user consent to upload videos and
  manage captions on personal or brand channels. Google does NOT permit
  standard Service Accounts to upload videos to typical YouTube channels.

- How talk_cut handles authentication:
  talk_cut acts as a desktop client. It starts a temporary local HTTP
  listener (http://127.0.0.1:<port>/oauth2callback), launches your system
  web browser to Google's sign-in page, receives the authorization code,
  and securely saves the refresh token locally.

- Default file locations:
  * Client secrets:  ~/.config/auth/youtube_client_secrets.json
  * Channel tokens:  ~/.config/auth/youtube_<channel>.json
  * talk_cut config: ~/.config/talk_cut/config.json

--------------------------------------------------------------------------------
2. STEP-BY-STEP GOOGLE CLOUD CONFIGURATION
--------------------------------------------------------------------------------

Step 2.1: Create or Select a Google Cloud Project
  1. Open Google Cloud Console: https://console.cloud.google.com/
  2. Create a new project (e.g. "Talk Publisher") or select an existing one.
  3. (Optional via gcloud):
     $ gcloud projects create talk-publisher-123
     $ gcloud config set project talk-publisher-123

Step 2.2: Enable the YouTube Data API v3
  1. In Cloud Console, navigate to "APIs & Services" > "Library".
  2. Search for "YouTube Data API v3" and click "Enable".
  3. (Optional via gcloud):
     $ gcloud services enable youtube.googleapis.com

Step 2.3: Configure the OAuth Consent Screen
  1. Go to "APIs & Services" > "OAuth consent screen".
  2. Select User Type: "External" (or "Internal" if using Google Workspace org).
  3. Click "Create".
  4. Fill in required fields:
     - App name: talk_cut
     - User support email: your email
     - Developer contact information: your email
  5. Scopes (click "Add or Remove Scopes"):
     - Filter and check:
       * .../auth/youtube.upload     (Upload YouTube videos)
       * .../auth/youtube.force-ssl  (Manage YouTube captions)
  6. Test Users (CRITICAL STEP):
     - While the app status is "Testing", Google blocks all authorizations
       with a 403 "access_denied" error UNLESS your Google account is added!
     - Click "+ ADD USERS" under "Test users".
     - Add the Gmail / Google account that owns the YouTube channel.
  7. Save and continue.

Step 2.4: Create OAuth Client ID (Desktop Application)
  1. Go to "APIs & Services" > "Credentials".
  2. Click "+ CREATE CREDENTIALS" at the top and select "OAuth client ID".
  3. Under "Application type", select "Desktop app".
     (IMPORTANT: Do NOT select "Web application").
  4. Name: "talk_cut Desktop Client".
  5. Click "Create".
  6. In the pop-up modal, click "DOWNLOAD JSON" to save the secrets file.

Step 2.5: Place the Client Secrets File
  Copy the downloaded JSON file to the default location:
  $ mkdir -p ~/.config/auth
  $ mv ~/Downloads/client_secret_*.json ~/.config/auth/youtube_client_secrets.json
  $ chmod 600 ~/.config/auth/youtube_client_secrets.json

--------------------------------------------------------------------------------
3. RUNNING GUIDED SETUP IN TALK_CUT
--------------------------------------------------------------------------------

Once the secrets file is in place, run:
  $ talk_cut youtube setup

This will interactively:
  1. Verify the client secrets file.
  2. Ask for your channel profile name (default: "default").
  3. Open your browser for one-click Google authentication.
  4. Save the refresh token to ~/.config/auth/youtube_<channel>.json.
  5. Register the channel in ~/.config/talk_cut/config.json.

Direct authorization command (non-interactive):
  $ talk_cut auth --channel seminar
  $ talk_cut youtube auth --channel seminar

--------------------------------------------------------------------------------
4. MULTI-CHANNEL PROFILES
--------------------------------------------------------------------------------

You can manage multiple YouTube channels (e.g. personal, seminar, course):

  $ talk_cut youtube setup --channel seminar
  $ talk_cut youtube setup --channel course

Your config at ~/.config/talk_cut/config.json will look like:
  {
    "default_channel": "seminar",
    "channels": {
      "seminar": "~/.config/auth/youtube_seminar.json",
      "course": "~/.config/auth/youtube_course.json"
    }
  }

Check configured channels and token status at any time:
  $ talk_cut youtube status

--------------------------------------------------------------------------------
5. PUBLISHING TALKS
--------------------------------------------------------------------------------

CLI Upload:
  $ talk_cut --upload talk_directory/
  $ talk_cut --upload --channel course talk_directory/

Interactive TUI:
  In the talk_cut interface, navigate to Tab 4 (YouTube / Publish).
  Here you can review title, description, adjusted chapter timestamps,
  select privacy (unlisted, public, private), and press Upload.

--------------------------------------------------------------------------------
6. TROUBLESHOOTING & FAQ
--------------------------------------------------------------------------------

- Error: "Access blocked: This app has not been verified" (403 access_denied)
  Fix: Your app is in "Testing" mode on Google Cloud Console. You must add
  your Google account email to the "Test users" list under
  "APIs & Services" > "OAuth consent screen".

- Error: "redirect_uri_mismatch"
  Fix: The OAuth credential was created as "Web application" instead of
  "Desktop app". Delete it and recreate as "Desktop app".

- Quotas:
  The default Google Cloud quota is 10,000 units/day. Uploading a video
  consumes ~1,600 units, allowing ~6 uploads per day on the free quota.

- Re-authorization:
  Tokens automatically refresh using offline refresh tokens. If a token
  is revoked or expired, simply run 'talk_cut youtube setup' again.
================================================================================`

// PrintDetailedSetupGuide outputs the comprehensive YouTube setup guide.
func PrintDetailedSetupGuide(w io.Writer) {
	fmt.Fprintln(w, setupGuideText)
}
