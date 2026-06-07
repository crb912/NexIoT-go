"""
NOTE: EdgeX Profile JSON Format differences (Common Pitfall):
EdgeX requires different JSON formats depending on how the profile is loaded:

1. Upload via API (Python/curl):
    The JSON must be an array and wrapped with the API version: [{"apiVersion": "v2", "profile": {...}}]

 2. Auto-load pre-defined profiles at startup:
    When the SDK reads directly from the local folder (res/profiles/),
    it expects a raw JSON object: {...}. Do NOT use the array or "profile" wrapper here.
"""

import json
import urllib.error
import urllib.request
from pathlib import Path


# Set the target URL
url = "http://localhost:59881/api/v2/deviceprofile"

# Get the folder where this script is saved
script_dir = Path(__file__).parent

# Find the profiles folder
profiles_dir = (script_dir / "../res/profiles").resolve()


def format_profile_for_api(raw_data: dict) -> list:
    """Wrap the raw device profile in the EdgeX V2 API format."""
    return [
        {
            "apiVersion": "v2",
            "profile": raw_data
        }
    ]

def update_device_profile(file_path: Path, url: str) -> None:
    """Read a JSON file, format it for the API, and send the update request."""
    print(f"Updating: {file_path.name}")

    try:
        # Open and read the JSON file as a string
        with open(file_path, 'r', encoding='utf-8') as f:
            file_data = json.load(f)

        # Check if data needs formatting
        # If it is a dictionary, we wrap it in the required array format
        if isinstance(file_data, dict):
            api_payload = format_profile_for_api(file_data)
        else:
            # If it is already a list, use it directly
            api_payload = file_data

        # Convert the Python dictionary back to JSON bytes
        payload_bytes = json.dumps(api_payload).encode('utf-8')

        # Create the HTTP request object
        req = urllib.request.Request(url, data=payload_bytes, method='PUT')
        req.add_header('Content-Type', 'application/json')

        # Send request to server
        with urllib.request.urlopen(req) as response:
            status_code = response.getcode()
            response_body = response.read().decode('utf-8')

            print(f"Status: {status_code}")
            print(f"Response: {response_body}")

    except json.JSONDecodeError:
        print(f"Error: File '{file_path}' is not valid JSON.")
    except FileNotFoundError:
        print(f"Error: File '{file_path}' not found.")
    except urllib.error.URLError as e:
        print(f"Network error: {e}")
    except Exception as e:
        print(f"An error occurred: {e}")


def main() -> None:
    """Find all JSON files in the folder and update them one by one."""
    # Check if the folder exists
    if not profiles_dir.exists() or not profiles_dir.is_dir():
        print(f"Error: Folder '{profiles_dir}' does not exist.")
        return

    print(f"Scanning folder: {profiles_dir}\n")

    # Find all JSON files in the folder
    json_files = list(profiles_dir.glob("*.json"))

    if not json_files:
        print("No JSON files found in the folder.")
        return

    # Update each profile
    for file_path in json_files:
        update_device_profile(file_path, url)
        print("-" * 40)


if __name__ == "__main__":
    main()