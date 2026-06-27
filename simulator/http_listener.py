import json
import logging
from http.server import BaseHTTPRequestHandler, HTTPServer
from typing import Dict, Any, List

config = {
  "ip": "127.0.0.1",
  "port": "8080",
  "business_name": "alarm",
  "device_id": "0_234",
  "device_name": "1号冷库设备"
}

api = '/alarm/push'

# Set up basic logging
def setup_logging() -> None:
    logging.basicConfig(
        level=logging.INFO,
        format='%(asctime)s - %(levelname)s - %(message)s'
    )

# Load configuration from JSON file
def load_config(file_path: str = "server_config.json") -> Dict[str, Any]:
    try:
        with open(file_path, 'r', encoding='utf-8') as file:
            return json.load(file)
    except FileNotFoundError:
        logging.warning("Config file not found. Using default settings.")
        return {"ip": "127.0.0.1", "port": 8080, "business_name": "alarm"}

# Check if the payload has the required structure
def is_valid_payload(payload: Dict[str, Any]) -> bool:
    if not payload or 'data' not in payload:
        return False
    if not isinstance(payload['data'], list):
        return False
    return True

# Extract and log a single alarm entry
def process_single_alarm(alarm_data: Dict[str, Any]) -> None:
    event_guid = alarm_data.get('event_guid', 'Unknown GUID')
    device_name = alarm_data.get('device_name', 'Unknown Device')
    content = alarm_data.get('event_content', 'No content provided')
    level = alarm_data.get('event_level', 'Unknown level')

    logging.info(
        f"Alarm Received | Level: {level} | "
        f"Device: {device_name} | GUID: {event_guid} | Content: {content}"
    )

# Loop through and process all alarms
def process_all_alarms(alarm_list: List[Dict[str, Any]]) -> int:
    processed_count = 0
    for alarm in alarm_list:
        process_single_alarm(alarm)
        processed_count += 1
    return processed_count

# Factory function to create the HTTP request handler with dynamic route
def make_handler_class(business_name: str):

    class AlarmRequestHandler(BaseHTTPRequestHandler):
        # Define the target URL path
        expected_path = api

        # Helper method to send HTTP response and JSON data
        def send_json_response(self, status_code: int, response_data: Dict[str, Any]) -> None:
            self.send_response(status_code)
            self.send_header('Content-Type', 'application/json;charset=utf-8')
            self.end_headers()
            response_bytes = json.dumps(response_data).encode('utf-8')
            self.wfile.write(response_bytes)

        # Handle incoming POST requests
        def do_POST(self) -> None:
            # Check if the URL path matches
            if self.path != self.expected_path:
                self.send_json_response(404, {"error": "Path not found"})
                return

            try:
                # Read the length of the incoming request body
                content_length = int(self.headers.get('Content-Length', 0))
                # Read the actual bytes
                post_data = self.rfile.read(content_length)
                # Parse bytes into a Python dictionary
                payload = json.loads(post_data.decode('utf-8'))
            except (ValueError, json.JSONDecodeError):
                logging.error("Failed to parse JSON payload.")
                self.send_json_response(400, {"error": "Invalid JSON format"})
                return

            # Check data validity
            if not is_valid_payload(payload):
                logging.error("Invalid payload structure.")
                self.send_json_response(400, {"error": "Invalid payload structure"})
                return

            # Process the alarms
            alarm_list = payload.get('data', [])
            count = process_all_alarms(alarm_list)

            # Return success message to client
            self.send_json_response(200, {"status": "success", "processed_count": count})

    return AlarmRequestHandler

# Initialize and start the HTTP server
def start_server(config: Dict[str, Any]) -> None:
    host = config.get("ip", "127.0.0.1")
    port = int(config.get("port", 8080))
    business_name = api

    # Generate the custom handler class
    handler_class = make_handler_class(business_name)
    server_address = (host, port)

    httpd = HTTPServer(server_address, handler_class)
    logging.info(f"Starting standard library server at http://{host}:{port}")
    logging.info("Press Ctrl+C to stop.")

    try:
        # Keep the server running
        httpd.serve_forever()
    except KeyboardInterrupt:
        logging.info("Shutting down server...")
        httpd.server_close()

# Main entry point
def main() -> None:
    setup_logging()
    # config = load_config()
    start_server(config)

if __name__ == '__main__':
    main()