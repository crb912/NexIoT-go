import json
import csv
import os

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
JSON_FILE_PATH = os.path.join(SCRIPT_DIR, "..", "res", "profiles", "modbus.test.profile.json")
CSV_FILE_PATH = os.path.join(SCRIPT_DIR, "..", "res", "custom", "csv_profiles","modbus.test.profile.csv")
JSON_FILE_PATH = os.path.abspath(JSON_FILE_PATH)
CSV_FILE_PATH = os.path.abspath(CSV_FILE_PATH)

def convert_json_to_csv():
    # Read the JSON file
    with open(JSON_FILE_PATH, 'r', encoding='utf-8') as json_file:
        data = json.load(json_file)

    # Add Command_Mappings to the CSV headers
    headers = [
        "Profile_Name", "Profile_Manufacturer", "Profile_Model",
        "Profile_Labels", "Profile_Description",
        "Resource_Name", "Resource_IsHidden", "Resource_Description",
        "Attr_PrimaryTable", "Attr_Address", "Attr_Length", "Attr_DecodeFunc",
        "Prop_ValueType", "Prop_ReadWrite", "Prop_Minimum", "Prop_Maximum", "Prop_DefaultValue",
        "Command_Name", "Command_ReadWrite", "Command_IsHidden", "Command_Mappings"
    ]

    # Map each resource to its command, including mappings
    resource_to_command = {}
    for cmd in data.get("deviceCommands", []):
        for op in cmd.get("resourceOperations", []):
            res_name = op.get("deviceResource")
            mappings = op.get("mappings", {})

            resource_to_command[res_name] = {
                "Command_Name": cmd.get("name", ""),
                "Command_ReadWrite": cmd.get("readWrite", ""),
                "Command_IsHidden": str(cmd.get("isHidden", False)).lower(),
                # Convert the mappings dictionary to a string for the CSV cell
                "Command_Mappings": json.dumps(mappings) if mappings else ""
            }

    # Ensure output directory exists before writing
    os.makedirs(os.path.dirname(CSV_FILE_PATH), exist_ok=True)

    # Write to the CSV file
    with open(CSV_FILE_PATH, 'w', newline='', encoding='utf-8') as csv_file:
        writer = csv.DictWriter(csv_file, fieldnames=headers)
        writer.writeheader()

        profile_name = data.get("name", "")
        profile_manufacturer = data.get("manufacturer", "")
        profile_model = data.get("model", "")
        profile_labels = ",".join(data.get("labels", []))
        profile_desc = data.get("description", "")

        for res in data.get("deviceResources", []):
            res_name = res.get("name", "")
            attrs = res.get("attributes", {})
            props = res.get("properties", {})

            # Default empty command info if not found
            cmd_info = resource_to_command.get(res_name, {
                "Command_Name": "",
                "Command_ReadWrite": "",
                "Command_IsHidden": "",
                "Command_Mappings": ""
            })

            row = {
                "Profile_Name": profile_name,
                "Profile_Manufacturer": profile_manufacturer,
                "Profile_Model": profile_model,
                "Profile_Labels": profile_labels,
                "Profile_Description": profile_desc,

                "Resource_Name": res_name,
                "Resource_IsHidden": str(res.get("isHidden", False)).lower(),
                "Resource_Description": res.get("description", ""),

                "Attr_PrimaryTable": attrs.get("primaryTable", ""),
                "Attr_Address": attrs.get("address", ""),
                "Attr_Length": attrs.get("length", ""),
                "Attr_DecodeFunc": attrs.get("decodefunc", ""),

                "Prop_ValueType": props.get("valueType", ""),
                "Prop_ReadWrite": props.get("readWrite", ""),
                "Prop_Minimum": props.get("minimum", ""),
                "Prop_Maximum": props.get("maximum", ""),
                "Prop_DefaultValue": props.get("defaultValue", "")
            }

            row.update(cmd_info)
            writer.writerow(row)

    print(f"Successfully converted JSON to CSV: {CSV_FILE_PATH}")

if __name__ == "__main__":
    convert_json_to_csv()