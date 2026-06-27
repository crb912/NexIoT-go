import json
import time
import uuid
import requests

config = {
  "ip": "127.0.0.1",
  "port": "8000",
  "device_id": "0_234",
  "device_name": "1号冷库设备"
}

# Build the URL dynamically from config
url = f"http://{config['ip']}:{config['port']}/alarm/push"

# Set headers as defined in the API docs
headers = {
    "Content-Type": "application/json;charset=utf-8",
    "Accept-Encoding": "gzip"
}

# Load the config file to avoid hardcoded values
def load_config(file_path="config.json"):
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            return json.load(f)
    except FileNotFoundError:
        print("Error: config.json file not found.")
        exit(1)

# Create a mock alarm payload matching the API structure
def generate_payload(config):
    # Get current Unix timestamp in seconds
    current_time = int(time.time())

    # Generate a unique GUID for the event
    event_guid = str(uuid.uuid4())

    payload = {
        "data": [
            {
                "device_id": config.get("device_id", "default_id"),
                "device_name": config.get("device_name", "default_device"),
                "point_id": "0_324_1_1_3",
                "point_name": "03温度传感器",
                "point_value": "-2.75",
                "point_type": 10,
                "event_guid": event_guid,
                "event_time": current_time,
                "event_type": 1,
                "event_content": "03温度低于下限阈值，当前值 -2.75℃",
                "event_level": 2,
                "event_location": "project_root/0_192/0_194/0_215/0_324",
                "event_location_msg": "办公区母线监控/办公区基础设施监控/G栋/B1F冷库/TH-G-LK-01",
                "event_suggest": "建议处理"
            }
        ],
                        "device_id": config.get("device_id", "default_id"),
                        "device_name": config.get("device_name", "default_device"),
                        "point_id": "0_324_1_1_3",
                        "point_name": "03温度传感器",
                        "point_value": "-2.75",
                        "point_type": 10,
                        "event_guid": event_guid,
                        "event_time": current_time,
                        "event_type": 1,
                        "event_content": "03温度低于下限阈值，当前值 -2.75℃",
                        "event_level": 2,
                        "event_location": "project_root/0_192/0_194/0_215/0_324",
                        "event_location_msg": "办公区母线监控/办公区基础设施监控/G栋/B1F冷库/TH-G-LK-01",
                        "event_suggest": "建议处理"
    }
    return payload

# Send the HTTP POST request to the server
def push_alarm(config, payload):
    try:
        # Send request with a 5-second timeout
        print(f'url: {url}')
        response = requests.post(url, headers=headers, json=payload, timeout=5)
        print(f'response : {response}')
        # Check if the request was successful
        if response.status_code == 200:
            print(f"[{time.strftime('%X')}] Success: Alarm pushed. GUID: {payload['data'][0]['event_guid']}")
        else:
            print(f"[{time.strftime('%X')}] Failed: HTTP {response.status_code} - {response.text}")

    except requests.exceptions.RequestException as e:
        print(f"[{time.strftime('%X')}] Network error: {e}")

# Main loop
def main():
    # config = load_config()
    print("Starting alarm push client. Press Ctrl+C to exit.")

    while True:
        payload = generate_payload(config)
        push_alarm(config, payload)

        # Pause the script for 20 seconds
        time.sleep(20)

if __name__ == "__main__":
    main()