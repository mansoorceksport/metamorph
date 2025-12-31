#!/bin/bash

# Simple E2E Test - Verify Account Linking
# Run this after test_e2e_email_provisioning.sh creates the user

set -e

API_URL="http://localhost:8080"

echo "🧪 Email Provisioning Verification Test"
echo "========================================"
echo ""

# The email from the previous test
ADMIN_EMAIL="test-admin-1766929110@homgym.app"
TENANT_ID="695132d6510f3f5906fd025c"
USER_ID="695132d6510f3f5906fd025d"

echo "Testing account linking for: $ADMIN_EMAIL"
echo ""

# Step 1: Get Firebase token (this will use get_firebase_token.py which signs in as this email)
echo "📝 Step 1: Signing in to Firebase as $ADMIN_EMAIL..."
echo "   (Using get_firebase_token.py which is configured for this email)"
FIREBASE_OUTPUT=$(python3 get_firebase_token.py 2>&1)
FIREBASE_TOKEN=$(echo "$FIREBASE_OUTPUT" | grep -A 1 "Your ID Token" | tail -1 | tr -d ' ')

if [ -z "$FIREBASE_TOKEN" ] || [[ ! "$FIREBASE_TOKEN" =~ ^eyJ ]]; then
    echo "❌ Failed to get Firebase token"
    exit 1
fi

# Extract Firebase UID
FIREBASE_UID=$(echo $FIREBASE_TOKEN | cut -d'.' -f2 | base64 -d 2>/dev/null | jq -r '.user_id')
echo "✅ Firebase token obtained (UID: $FIREBASE_UID)"
echo ""

# Step 2: Login to Metamorph (should link account)
echo "📝 Step 2: Logging in to Metamorph..."
LOGIN_RESPONSE=$(curl -s -X POST "$API_URL/v1/auth/login" \
  -H "Authorization: Bearer $FIREBASE_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"requested_role": "tenant_admin"}')

USER_ID_RESPONSE=$(echo $LOGIN_RESPONSE | jq -r '.user.id')
FIREBASE_UID_RESPONSE=$(echo $LOGIN_RESPONSE | jq -r '.user.firebase_uid')
TENANT_ID_RESPONSE=$(echo $LOGIN_RESPONSE | jq -r '.user.tenant_id')
ROLES_RESPONSE=$(echo $LOGIN_RESPONSE | jq -r '.user.roles | join(", ")')
IS_NEW=$(echo $LOGIN_RESPONSE | jq -r '.is_new_user')

echo "✅ Logged in"
echo ""

# Verification
echo "📋 Verification Results"
echo "========================================"

SUCCESS=true

# Check 1
echo "🔍 User ID: $USER_ID_RESPONSE"
if [ "$USER_ID_RESPONSE" = "$USER_ID" ]; then
    echo "   ✅ Matches pre-provisioned user"
else
    echo "   ❌ MISMATCH! Expected: $USER_ID"
    SUCCESS=false
fi

# Check 2
echo "🔍 Firebase UID: $FIREBASE_UID_RESPONSE"
if [ -n "$FIREBASE_UID_RESPONSE" ] && [ "$FIREBASE_UID_RESPONSE" != "" ] && [ "$FIREBASE_UID_RESPONSE" != "null" ]; then
    echo "   ✅ Firebase UID linked: $FIREBASE_UID_RESPONSE"
else
    echo "   ❌ Firebase UID NOT linked"
    SUCCESS=false
fi

# Check 3
echo "🔍 Tenant ID: $TENANT_ID_RESPONSE"
if [ "$TENANT_ID_RESPONSE" = "$TENANT_ID" ]; then
    echo "   ✅ Correct tenant"
else
    echo "   ❌ Wrong tenant! Expected: $TENANT_ID"
    SUCCESS=false
fi

# Check 4
echo "🔍 Roles: $ROLES_RESPONSE"
if [[ "$ROLES_RESPONSE" == *"tenant_admin"* ]]; then
    echo "   ✅ Has tenant_admin role"
else
    echo "   ❌ Missing tenant_admin role"
    SUCCESS=false
fi

# Check 5
echo "🔍 Is New User: $IS_NEW"
if [ "$IS_NEW" = "false" ]; then
    echo "   ✅ Recognized as existing user"
else
    echo "   ❌ Flagged as new user (should be false)"
    SUCCESS=false
fi

echo ""
if [ "$SUCCESS" = true ]; then
    echo "🎉 ALL CHECKS PASSED!"
    echo "✅ Email-based provisioning working!"
    echo "✅ Account linking successful!"
else
    echo "❌ Some checks failed"
    exit 1
fi
