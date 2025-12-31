#!/usr/bin/env python3
import sys
import json
import requests

if len(sys.argv) < 2:
    print("Usage: python3 firebase_signin.py <email> [password]", file=sys.stderr)
    sys.exit(1)

EMAIL = sys.argv[1]
PASSWORD = sys.argv[2] if len(sys.argv) > 2 else "123456"
API_KEY = "AIzaSyBSLjVinF_VdJOQwtQxNqD7TVBj0wCJR60"

# Try to sign in first
url = f"https://identitytoolkit.googleapis.com/v1/accounts:signInWithPassword?key={API_KEY}"
response = requests.post(url, json={
    "email": EMAIL,
    "password": PASSWORD,
    "returnSecureToken": True
})

# If sign in fails, try to sign up
if response.status_code != 200:
    url = f"https://identitytoolkit.googleapis.com/v1/accounts:signUp?key={API_KEY}"
    response = requests.post(url, json={
        "email": EMAIL,
        "password": PASSWORD,
        "returnSecureToken": True
    })

if response.status_code == 200:
    data = response.json()
    print(data["idToken"])
else:
    print(f"Error: {response.text}", file=sys.stderr)
    sys.exit(1)
