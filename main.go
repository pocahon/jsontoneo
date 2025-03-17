package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"gopkg.in/yaml.v2"
)

// Struct voor de Neo4j configuratie
type Neo4jConfig struct {
	URI      string `yaml:"uri"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Structs om JSON data op te slaan
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
	Host      string   `json:"host"` // Dit veld bevat het IP-adres
	Status    int      `json:"status_code"`
	Words     int      `json:"words"`
	Lines     int      `json:"lines"`
	Resolvers []string `json:"resolvers"`
}

func main() {
	// CLI-parameter voor het JSON bestand (JSON Lines formaat wordt verwacht)
	filePath := flag.String("f", "", "Path to the JSON file (JSON Lines format expected)")
	flag.Parse()

	if *filePath == "" {
		log.Fatal("Usage: go run main.go -f <path to JSON file>")
	}

	// Bepaal de configuratie-locatie
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Error getting user home directory: %v", err)
	}
	configDir := filepath.Join(home, ".config", "jsontoneo")
	configPath := filepath.Join(configDir, "neo4j_config.yaml")

	var config Neo4jConfig

	// Als het config-bestand nog niet bestaat, maak de map aan en vraag de credentials op
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		err = os.MkdirAll(configDir, 0700)
		if err != nil {
			log.Fatalf("Error creating config directory: %v", err)
		}

		reader := bufio.NewReader(os.Stdin)

		fmt.Print("Enter Neo4j URI [default neo4j://localhost:7687]: ")
		uriInput, _ := reader.ReadString('\n')
		uriInput = strings.TrimSpace(uriInput)
		if uriInput == "" {
			uriInput = "neo4j://localhost:7687"
		}

		fmt.Print("Enter Neo4j Username [default neo4j]: ")
		usernameInput, _ := reader.ReadString('\n')
		usernameInput = strings.TrimSpace(usernameInput)
		if usernameInput == "" {
			usernameInput = "neo4j"
		}

		fmt.Print("Enter Neo4j Password [default neo4jpass]: ")
		passwordInput, _ := reader.ReadString('\n')
		passwordInput = strings.TrimSpace(passwordInput)
		if passwordInput == "" {
			passwordInput = "neo4jpass"
		}

		config = Neo4jConfig{
			URI:      uriInput,
			Username: usernameInput,
			Password: passwordInput,
		}

		yamlData, err := yaml.Marshal(&config)
		if err != nil {
			log.Fatalf("Error marshalling YAML: %v", err)
		}

		err = os.WriteFile(configPath, yamlData, 0600)
		if err != nil {
			log.Fatalf("Error writing config file: %v", err)
		}
		fmt.Printf("Configuration file created at %s\n", configPath)
	} else {
		// Lees de configuratie uit het bestand
		yamlData, err := os.ReadFile(configPath)
		if err != nil {
			log.Fatalf("Error reading config file: %v", err)
		}
		err = yaml.Unmarshal(yamlData, &config)
		if err != nil {
			log.Fatalf("Error parsing config file: %v", err)
		}
	}

	// Open het JSON bestand
	file, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("Error opening JSON file: %v", err)
	}
	defer file.Close()

	// Maak verbinding met Neo4j met de credentials uit het configuratiebestand
	driver, err := neo4j.NewDriver(config.URI, neo4j.BasicAuth(config.Username, config.Password, ""))
	if err != nil {
		log.Fatalf("Error connecting to Neo4j: %v", err)
	}
	defer driver.Close()

	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var result HttpxResult
		if err := json.Unmarshal(scanner.Bytes(), &result); err != nil {
			log.Printf("Error parsing JSON: %v", err)
			continue
		}

		log.Printf("Processing URL: %s", result.URL)

		_, err := session.WriteTransaction(func(tx neo4j.Transaction) (any, error) {
			// Maak of update de Host node en sla deze op
			hostQuery := `
			MERGE (h:Host {url: $url})
			SET h.input = $input,
				h.port = $port,
				h.title = $title,
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

			// Voeg de IP node toe en maak de relatie met de Host
			ipQuery := `
			MATCH (h:Host {url: $url})
			MERGE (i:IP {address: $ip})
			MERGE (h)-[:RESOLVES_TO]->(i)
			`
			_, err = tx.Run(ipQuery, map[string]any{
				"url": result.URL,
				"ip":  result.Host,
			})
			if err != nil {
				return nil, fmt.Errorf("IP query error: %w", err)
			}

			// Voeg Tech nodes toe en maak de relaties
			for _, tech := range result.Tech {
				techQuery := `
				MATCH (h:Host {url: $url})
				MERGE (t:Tech {name: $tech})
				MERGE (h)-[:USES]->(t)
				`
				_, err = tx.Run(techQuery, map[string]any{
					"url":  result.URL,
					"tech": tech,
				})
				if err != nil {
					return nil, fmt.Errorf("Tech query error: %w", err)
				}
			}

			// Voeg ASN data toe als beschikbaar
			if result.ASN.ASNumber != "" {
				asnQuery := `
				MATCH (h:Host {url: $url})
				MERGE (a:ASN {number: $as_number})
				SET a.name = $as_name, a.country = $as_country
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

	fmt.Println("JSON data successfully processed into Neo4j!")
}
