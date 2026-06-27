# Requirements:  pip install pyyaml

import json
import urllib.error
import urllib.request
from pathlib import Path
import yaml

# API endpoint URL
url = "http://localhost:59881/api/v2/device"

script_dir = Path(__file__).parent
devices_dir = (script_dir / "../res/devices").resolve()

# Default fields to add to each device
padding_data = {
    "adminState": "UNLOCKED",
    "operatingState": "UP",
    "serviceName": "devices-iot-go"
}

def wrap_devices(file_data) -> list:
    """Wrap raw device data into EdgeX V2 API format."""
    if isinstance(file_data, dict):
        # Convert single device to list
        items = [file_data]
    elif isinstance(file_data, list):
        items = file_data
    else:
        raise ValueError(f"Unexpected data structure: {type(file_data)}")

    # Merge padding_data into each device dictionary
    for item in items:
        item.update(padding_data)

    return [{"apiVersion": "v2", "device": item} for item in items]

def update_device(file_data, url: str) -> None:
    """Send device data to the API."""
    try:
        api_payload = wrap_devices(file_data)
        payload_bytes = json.dumps(api_payload).encode('utf-8')
        req = urllib.request.Request(url, data=payload_bytes, method='POST')
        req.add_header('Content-Type', 'application/json')

        with urllib.request.urlopen(req) as response:
            status_code = response.getcode()
            response_body = response.read().decode('utf-8')
            print(f"Status: {status_code}")
            print(f"Response: {response_body}")

    except Exception as e:
        print(f"An error occurred: {e}")
        print(api_payload)

def update_json_devices():
    """Scan and update JSON files."""
    json_files = list(devices_dir.glob("*.json"))
    for file_path in json_files:
        print(f"Updating device (json): {file_path}")
        with open(file_path, 'r', encoding='utf-8') as f:
            file_data = json.load(f)
            update_device(file_data, url)
            print("-" * 40)

def read_yaml_file(file_path: Path):
    """Read and parse YAML file."""
    print(f"Reading YAML file: {file_path}")
    with open(file_path, 'r', encoding='utf-8') as f:
        data = yaml.safe_load(f)

        # Extract device list if the key 'deviceList' exists
        if isinstance(data, dict) and 'deviceList' in data:
            return data['deviceList']
        return data

def update_yaml_devices():
    """Scan and update YAML/YML files."""
    yaml_files = list(devices_dir.glob("*.yaml"))
    yaml_files.extend(list(devices_dir.glob("*.yml")))

    for file_path in yaml_files:
        data = read_yaml_file(file_path)
        print(f"Updating device (yaml): {file_path}")
        update_device(data, url)
        print("-" * 40)

def main() -> None:
    """Main function."""
    if not devices_dir.exists() or not devices_dir.is_dir():
        print(f"Error: Folder '{devices_dir}' does not exist.")
        return

    print(f"Scanning folder: {devices_dir}\n")
    update_json_devices()
    update_yaml_devices()

if __name__ == "__main__":
    main()