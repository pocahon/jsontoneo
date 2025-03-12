package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"gopkg.in/yaml.v3"
)

// Structs to store JSON data
type ASN struct {
	ASNumber  string   `json:"as_number"`
	ASName    string   `json:"as_name"`
	ASCountry string   `json:"as_country"`
	ASRange   []string `json:"as_range"`
}

type HttpxResult struct {
	Timestamp string   `json:"timestamp"`
	ASN       ASN      `json:"asn"`
	Port      string   `json:"port"`
	URL       string   `json:"url"`
	Input     string   `json:"input"`
	Title     string   `json:"title"`
	Scheme    string   `json:"scheme"`
	Webserver string   `json:"webserver"`
	Tech      []string `json:"tech"`
	Host      string   `json:"host"`
	Status    int      `json:"status_code"`
	Words     int      `json:"words"`
	Lines     int      `json:"lines"`
	Resolvers []string `json:"resolvers"`
}

// Struct for the Neo4j configuration
type Neo4jConfig struct {
	Neo4j struct {
		URI      string `yaml:"uri"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
	} `yaml:"neo4j"`
}

// Function to create the config file if it doesn't exist
func createConfigIfNotExist() {
	// Path to the configuration file
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "jsontoneo")
	configPath := filepath.Join(configDir, "neo4j-config.yaml")

	// Check if the file already exists
	if _, err := os.Stat(configPath); err == nil {
		fmt.Println("Config file already exists, skipping creation.")
		return
	}

	// Create the directory if it doesn't exist
	if err := os.MkdirAll(configDir, os.ModePerm); err != nil {
		log.Fatalf("Failed to create config directory: %v", err)
	}

	// Set default config
	defaultConfig := Neo4jConfig{
		Neo4j: struct {
			URI      string `yaml:"uri"`
			User     string `yaml:"user"`
			Password string `yaml:"password"`
		}{
			URI:      "neo4j://localhost:7687",
			User:     "neo4j",
			Password: "neo4jpass",
		},
	}

	// Marshal the config into YAML
	configData, err := yaml.Marshal(&defaultConfig)
	if err != nil {
		log.Fatalf("Failed to marshal config: %v", err)
	}

	// Write the config to the file
	if err := ioutil.WriteFile(configPath, configData, 0644); err != nil {
		log.Fatalf("Failed to write config file: %v", err)
	}

	fmt.Println("Config file created at:", configPath)
}

func main() {
	// Create the config file if it doesn't exist
	createConfigIfNotExist()

	// CLI parameter for the file
	filePath := flag.String("f", "", "Path to the JSON file")
	flag.Parse()

	// Check if a file was provided
	if *filePath == "" {
		log.Fatal("Usage: go run main.go -f <path to json file>")
	}

	// Open the provided file
	file, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("Error opening JSON file: %v", err)
	}
	defer file.Close()

	// Load Neo4j credentials from the config file
	configFilePath := filepath.Join(os.Getenv("HOME"), ".config", "jsontoneo", "neo4j-config.yaml")
	configData, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}

	var config Neo4jConfig
	if err := yaml.Unmarshal(configData, &config); err != nil {
		log.Fatalf("Failed to unmarshal config: %v", err)
	}

	// Connect to Neo4j
	driver, err := neo4j.NewDriver(config.Neo4j.URI, neo4j.BasicAuth(config.Neo4j.User, config.Neo4j.Password, ""))
	if err != nil {
		log.Fatalf("Error connecting to Neo4j: %v", err)
	}
	defer driver.Close()

	// Create session without context
	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()

	// Process JSON line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var result HttpxResult
		if err := json.Unmarshal(scanner.Bytes(), &result); err != nil {
			log.Printf("Error parsing JSON: %v", err)
			continue
		}

		log.Printf("Processing URL: %s", result.URL)

		_, err := session.WriteTransaction(func(tx neo4j.Transaction) (any, error) {
			// Add Host
			hostQuery := `
            MERGE (h:Host {url: $url})
            SET h.input = $input,
                h.port = $port,
                h.title = $url,
                h.scheme = $scheme,
                h.webserver = $webserver,
                h.status = $status,
                h.words = $words,
                h.lines = $lines
            RETURN h
            `
			_, err := tx.Run(hostQuery, map[string]any{
				"url":       result.URL,
				"input":     result.Input,
				"port":      result.Port,
				"title":     result.Title,
				"scheme":    result.Scheme,
				"webserver": result.Webserver,
				"status":    result.Status,
				"words":     result.Words,
				"lines":     result.Lines,
			})
			if err != nil {
				return nil, fmt.Errorf("Host query error: %w", err)
			}

			// Add IP and relationship with Host
			ipQuery := `
            MERGE (i:IP {address: $ip})
            MERGE (h:Host {url: $url})
            MERGE (h)-[:RESOLVES_TO]->(i)
            `
			_, err = tx.Run(ipQuery, map[string]any{
				"ip":  result.Host,
				"url": result.URL,
			})
			if err != nil {
				return nil, fmt.Errorf("IP query error: %w", err)
			}

			// Add Tech nodes and relationships
			for _, tech := range result.Tech {
				techQuery := `
                MERGE (t:Tech {name: $tech})
                MERGE (h:Host {url: $url})
                MERGE (h)-[:USES]->(t)
                `
				_, err := tx.Run(techQuery, map[string]any{
					"tech": tech,
					"url":  result.URL,
				})
				if err != nil {
					return nil, fmt.Errorf("Tech query error: %w", err)
				}
			}

			// Add ASN data
			asnQuery := `
            MERGE (a:ASN {number: $as_number})
            SET a.name = $as_name, a.country = $as_country
            MERGE (h:Host {url: $url})
            MERGE (h)-[:BELONGS_TO]->(a)
            `
			_, err = tx.Run(asnQuery, map[string]any{
				"as_number":  result.ASN.ASNumber,
				"as_name":    result.ASN.ASName,
				"as_country": result.ASN.ASCountry,
				"url":        result.URL,
			})
			if err != nil {
				return nil, fmt.Errorf("ASN query error: %w", err)
			}
			return nil, nil
		})

		if err != nil {
			log.Printf("Error processing %s: %v", result.URL, err)
		} else {
			fmt.Printf("Added to Neo4j: %s\n", result.URL)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading file: %v", err)
	}

	fmt.Println("Finished processing JSON data to Neo4j!")
}
