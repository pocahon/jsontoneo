# Neo4j HTTPX Data Importer

This Go script processes **JSON output from HTTPX (Project Discovery)** and imports the data into a **Neo4j database**. It extracts information about **hosts, IP addresses, technologies, and ASN data**, structuring them into a graph database for further analysis.

## 🚀 Features
- **Processes JSON line by line** to minimize memory usage.  
- **Automates data import into Neo4j** for visualization and analysis.  
- **Creates relationships** between Hosts, IPs, Technologies, and ASN data.  
- **Supports command-line arguments** for flexible file input.  

## 🛠️ Installation & Usage

### 1. Installation
Install directly using Go and remember that the password in the main.go file needs to be changed to the password of your Neo4j setup:  
```sh
go install github.com/pocahon/jsontoneo@latest
