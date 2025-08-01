import requests
import json
import uuid
import time

# config
BASE_URL = "http://localhost:80"
TEST_EMAIL = f"test_{uuid.uuid4().hex[:8]}@example.com"
TEST_PASSWORD = "BIGsmal123#"
DEVICE_ID = str(uuid.uuid4())

# helper functions
def print_response(response):
    print(f"Status: {response.status_code}")
    try:
        print(json.dumps(response.json(), indent=2))
    except:
        print(response.text)

def register_user(email, password):
    url = f"{BASE_URL}/register"
    payload = {
        "email": email,
        "password": password
    }
    response = requests.post(url, json=payload)
    return response

def login_user(email, password, device_id):
    url = f"{BASE_URL}/login"
    payload = {
        "email": email,
        "password": password,
        "device_id": device_id
    }
    response = requests.post(url, json=payload)
    return response

def refresh_token(refresh_token, device_id):
    url = f"{BASE_URL}/refresh"
    headers = {
        "Authorization": f"Bearer {refresh_token}",
        "X-Device-ID": device_id
    }
    response = requests.post(url, headers=headers)
    return response

def protected_request(access_token, device_id):
    url = f"{BASE_URL}/debug"
    headers = {
        "Authorization": f"Bearer {access_token}",
        "X-Device-ID": device_id
    }
    response = requests.get(url, headers=headers)
    return response

def logout_user(access_token, device_id):
    url = f"{BASE_URL}/logout"
    headers = {
        "Authorization": f"Bearer {access_token}",
        "X-Device-ID": device_id
    }
    response = requests.post(url, headers=headers)
    return response

def test_full_authentication_flow():
    print("\n=== Testing Full Authentication Flow ===")
    
    print("\n[1] Registering new user...")
    register_response = register_user(TEST_EMAIL, TEST_PASSWORD)
    print_response(register_response)
    assert register_response.status_code == 201
    user_id = register_response.json().get("id")
    assert user_id is not None
    
    print("\n[2] Logging in...")
    login_response = login_user(TEST_EMAIL, TEST_PASSWORD, DEVICE_ID)
    print_response(login_response)
    assert login_response.status_code == 200
    
    access_token = login_response.json().get("access_token")
    refresh_token_value = login_response.json().get("refresh_token")
    assert access_token is not None
    assert refresh_token_value is not None
    
    print("\n[3] Accessing protected route...")
    protected_response = protected_request(access_token, DEVICE_ID)
    print_response(protected_response)
    assert protected_response.status_code == 200
    
    print("\n[4] Simulating token expiration...")
    time.sleep(2)
    
    print("\n[5] Refreshing tokens...")
    refresh_response = refresh_token(refresh_token_value, DEVICE_ID)
    print_response(refresh_response)
    assert refresh_response.status_code == 200
    
    new_access_token = refresh_response.json().get("access_token")
    new_refresh_token = refresh_response.json().get("refresh_token")
    assert new_access_token is not None
    assert new_refresh_token is not None
    
    print("\n[6] Accessing protected route with new token...")
    protected_response = protected_request(new_access_token, DEVICE_ID)
    print_response(protected_response)
    assert protected_response.status_code == 200
    
    print("\n[7] Logging out...")
    logout_response = logout_user(new_access_token, DEVICE_ID)
    print_response(logout_response)
    assert logout_response.status_code == 200
    
    print("\n[8] Verifying token is invalid after logout...")
    protected_response = protected_request(new_access_token, DEVICE_ID)
    print_response(protected_response)
    assert protected_response.status_code == 401

def test_invalid_credentials():
    print("\n=== Testing Invalid Credentials ===")
    
    print("\n[1] Attempting login with wrong password...")
    response = login_user(TEST_EMAIL, "fürfortnite", DEVICE_ID)
    print_response(response)
    assert response.status_code == 401
    
    print("\n[2] Attempting with expired token...")
    response = protected_request("expired_token", DEVICE_ID)
    print_response(response)
    assert response.status_code != 200

def test_device_binding():
    print("\n=== Testing Device Binding ===")
    
    print("\n[1] Logging in from device 1...")
    device1_id = str(uuid.uuid4())
    login_response = login_user(TEST_EMAIL, TEST_PASSWORD, device1_id)
    print_response(login_response)
    assert login_response.status_code == 200
    
    refresh_token1 = login_response.json().get("refresh_token")
    
    print("\n[2] Attempting refresh from device 2...")
    device2_id = str(uuid.uuid4())
    response = refresh_token(refresh_token1, device2_id)
    print_response(response)
    assert response.status_code == 401

def test_concurrent_sessions():
    print("\n=== Testing Concurrent Sessions ===")
    
    print("\n[1] Creating multiple sessions...")
    devices = [str(uuid.uuid4()) for _ in range(3)]
    tokens = []
    
    for device_id in devices:
        response = login_user(TEST_EMAIL, TEST_PASSWORD, device_id)
        print_response(response)
        assert response.status_code == 200
        tokens.append(response.json().get("access_token"))

    print("\n[2] Verifying all sessions are active...")
    for i, token in enumerate(tokens):
        response = protected_request(token, devices[i])
        print_response(response)
        assert response.status_code == 200

if __name__ == "__main__":
    test_full_authentication_flow()
    test_invalid_credentials()
    test_device_binding()
    test_concurrent_sessions()
    
    print("\nyipieee!")