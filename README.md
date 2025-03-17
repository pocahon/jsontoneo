# JsonToNeo

This Go script processes **JSON output from HTTPX (Project Discovery)** and imports the data into a **Neo4j database**. It extracts information about **hosts, IP addresses, technologies, and ASN data**, structuring them into a graph database for further analysis.

## 🚀 Features
- **Processes JSON line by line** to minimize memory usage.  
- **Automates data import into Neo4j** for visualization and analysis.  
- **Creates relationships** between Hosts, IPs, Technologies, and ASN data.  
- **Supports command-line arguments** for flexible file input.  

## 🛠️ Installation & Usage

### 1. Installation
Install directly using Go:  
```sh
go install github.com/pocahon/jsontoneo@latest
```
### 2. Configuration

On the first run, the script checks for a configuration file at ~/.config/jsontoneo/neo4j_config.yaml. If the file does not exist, it automatically creates the necessary directory and prompts you to enter your Neo4j credentials (URI, username, and password). These details are then saved in the configuration file for subsequent runs.

The default configuration file, once created, will contain:
```sh
neo4j:
  uri: "neo4j://localhost:7687"
  user: "neo4j"
  password: "neo4jpass"
```
### 3. Usage

After installation, you can run the script by specifying the path to the JSON file you wish to process. The script will automatically import the data into your Neo4j database, creating nodes and relationships based on the extracted information.
```sh
jsontoneo -f /path/to/your/httpx-output.json
