import json
import csv
import os

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
JSON_FILE_PATH = os.path.join(SCRIPT_DIR, "..", "res", "custom", "json_profiles", "modbus.test.profile.json")
CSV_FILE_PATH = os.path.join(SCRIPT_DIR, "..", "res", "custom", "csv_profiles","modbus.test.profile.csv")
JSON_FILE_PATH = os.path.abspath(JSON_FILE_PATH)
CSV_FILE_PATH = os.path.abspath(CSV_FILE_PATH)

def convert_csv_to_json():
    profile_data = {}
    device_resources = []
    commands_dict = {}

    # Read the CSV file
    with open(CSV_FILE_PATH, 'r', encoding='utf-8') as csv_file:
        reader = csv.DictReader(csv_file)

        for i, row in enumerate(reader):
            # Get profile level data from the first row
            if i == 0:
                profile_data = {
                    "name": row["Profile_Name"],
                    "manufacturer": row["Profile_Manufacturer"],
                    "model": row["Profile_Model"],
                    "labels": [label.strip() for label in row["Profile_Labels"].split(",") if label.strip()],
                    "description": row["Profile_Description"]
                }

            # Build resource attributes
            attributes = {
                "primaryTable": row["Attr_PrimaryTable"],
                "address": int(row["Attr_Address"]) if row["Attr_Address"] else 0,
                "length": int(row["Attr_Length"]) if row["Attr_Length"] else 1
            }
            if row["Attr_DecodeFunc"]:
                attributes["decodeFunc"] = row["Attr_DecodeFunc"]

            # Build resource properties
            properties = {
                "valueType": row["Prop_ValueType"],
                "readWrite": row["Prop_ReadWrite"]
            }
            if row["Prop_Minimum"]:
                properties["minimum"] = row["Prop_Minimum"]
            if row["Prop_Maximum"]:
                properties["maximum"] = row["Prop_Maximum"]
            if row["Prop_DefaultValue"]:
                properties["defaultValue"] = row["Prop_DefaultValue"]

            # Add to device resources list
            resource = {
                "name": row["Resource_Name"],
                "isHidden": row["Resource_IsHidden"] == "true",
                "description": row["Resource_Description"],
                "attributes": attributes,
                "properties": properties
            }
            device_resources.append(resource)

            # Build device commands
            cmd_name = row.get("Command_Name")
            if cmd_name:
                if cmd_name not in commands_dict:
                    commands_dict[cmd_name] = {
                        "name": cmd_name,
                        "readWrite": row["Command_ReadWrite"],
                        "isHidden": row["Command_IsHidden"] == "true",
                        "resourceOperations": []
                    }

                # Build the operation for this specific resource
                operation = {
                    "deviceResource": row["Resource_Name"]
                }

                # Parse mappings from JSON string back to a Python dictionary
                mappings_str = row.get("Command_Mappings", "")
                if mappings_str:
                    try:
                        operation["mappings"] = json.loads(mappings_str)
                    except json.JSONDecodeError:
                        print(f"Warning: Could not parse mappings for {row['Resource_Name']}")

                commands_dict[cmd_name]["resourceOperations"].append(operation)

    # Put everything together
    profile_data["deviceResources"] = device_resources
    profile_data["deviceCommands"] = list(commands_dict.values())

    # Ensure output directory exists before writing
    os.makedirs(os.path.dirname(JSON_FILE_PATH), exist_ok=True)

    # Write the JSON file
    with open(JSON_FILE_PATH, 'w', encoding='utf-8') as json_file:
        json.dump(profile_data, json_file, indent=2, ensure_ascii=False)

    print(f"Successfully converted CSV to JSON: {JSON_FILE_PATH}")

if __name__ == "__main__":
    convert_csv_to_json()