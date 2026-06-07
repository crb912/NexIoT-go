import json
import urllib.error
import urllib.request
from pathlib import Path


# Target URL changed to the device endpoint
url = "http://localhost:59881/api/v2/device"

# Get the folder where this script is saved
script_dir = Path(__file__).parent

# Find the devices folder
devices_dir = (script_dir / "../res/devices").resolve()


def format_device_for_api(raw_data: dict) -> list:
    # Wrap the raw device data in the EdgeX V2 API format.
    return [
        {
            "apiVersion": "v2",
            "device": raw_data  # Changed from "profile" to "device"
        }
    ]


def update_device(file_path: Path, url: str) -> None:
    # Read a JSON file, format it for the API, and send the update request.
    print(f"Updating device: {file_path}")

    try:
        # Open and read the JSON file as a string
        with open(file_path, 'r', encoding='utf-8') as f:
            file_data = json.load(f)

        # Check if data needs formatting
        # If it is a dictionary, we wrap it in the required array format
        if isinstance(file_data, dict):
            api_payload = format_device_for_api(file_data)
        else:
            # If it is already a list, use it directly
            api_payload = file_data

        # Convert the Python dictionary back to JSON bytes
        payload_bytes = json.dumps(api_payload).encode('utf-8')

        # Create the HTTP request object
        #  Use POST if adding a new device.
        req = urllib.request.Request(url, data=payload_bytes, method='POST')
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
    except urllib.error.HTTPError as e:
        # Read the error body from the server to see what went wrong
        error_msg = e.read().decode('utf-8')
        print(f"HTTP Error {e.code}: {error_msg}")
    except urllib.error.URLError as e:
        print(f"Network error: {e}")
    except Exception as e:
        print(f"An error occurred: {e}")


def main() -> None:
    # Find all JSON files in the devices folder and update them one by one.

    # Check if the folder exists
    if not devices_dir.exists() or not devices_dir.is_dir():
        print(f"Error: Folder '{devices_dir}' does not exist.")
        return

    print(f"Scanning folder: {devices_dir}\n")

    # Find all JSON files in the folder
    json_files = list(devices_dir.glob("*.json"))

    if not json_files:
        print("No JSON files found in the folder.")
        return

    # Update each device
    for file_path in json_files:
        update_device(file_path, url)
        print("-" * 40)


if __name__ == "__main__":
    main()