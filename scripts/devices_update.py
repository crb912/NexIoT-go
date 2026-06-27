import json
import urllib.error
import urllib.request
from pathlib import Path


# Target URL for EdgeX Core Metadata Device API
url = "http://localhost:59881/api/v2/device"

# Get the folder where this script is saved
script_dir = Path(__file__).parent

# Find the devices folder
devices_dir = (script_dir / "../res/devices").resolve()


def format_device_payload(file_data) -> list:
    """Ensure the data is correctly wrapped in the EdgeX V2 API format."""

    # 1. 如果传入的是单个设备对象 {}，把它包装成标准数组
    if isinstance(file_data, dict):
        return [{"apiVersion": "v2", "device": file_data}]

    # 2. 如果传入的是数组 []
    if isinstance(file_data, list):
        if not file_data:
            return []

        # 检查数组里的第一个元素。如果已经有 apiVersion，说明是标准 API 格式，直接返回
        if "apiVersion" in file_data[0]:
            return file_data

        # 如果没有 apiVersion，说明这是本地用的裸数组配置，我们需要遍历包装每一个设备
        formatted_list = []
        for item in file_data:
            formatted_list.append({
                "apiVersion": "v2",
                "device": item
            })
        return formatted_list

    return file_data


def update_device(file_path: Path, url: str) -> None:
    """Read a JSON file, format it for the API, and send the request."""
    print(f"Updating device: {file_path.name}")

    try:
        # Open and read the JSON file as a string
        with open(file_path, 'r', encoding='utf-8') as f:
            file_data = json.load(f)

        # 智能转换格式，不再单纯依赖是否为 dict
        api_payload = format_device_payload(file_data)

        # Convert the Python dictionary back to JSON bytes
        payload_bytes = json.dumps(api_payload).encode('utf-8')

        # Use PATCH to update an existing device. (Use POST if creating for the first time)
        req = urllib.request.Request(url, data=payload_bytes, method='PATCH')
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
    """Find all JSON files in the devices folder and update them one by one."""
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

    # Update each device file
    for file_path in json_files:
        update_device(file_path, url)
        print("-" * 40)


if __name__ == "__main__":
    main()