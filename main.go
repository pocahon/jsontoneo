package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
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

// init function to create the neo4j-config.yaml file
func init() {
	// Pad naar de configuratiemap
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Error getting home directory: %v", err)
	}
	configDir := filepath.Join(homeDir, ".config", "jsontoneo")
	configFile := filepath.Join(configDir, "neo4j-config.yaml")

	// Controleer of de map bestaat, zo niet, maak deze aan
	if err := os.MkdirAll(configDir, 0755); err != nil {
		log.Fatalf("Error creating config directory: %v", err)
	}

	// Controleer of het bestand al bestaat
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// Maak het bestand aan als het nog niet bestaat
		fmt.Println("Creating neo4j config file at", configFile)
		file, err := os.Create(configFile)
		if err != nil {
			log.Fatalf("Error creating config file: %v", err)
		}
		defer file.Close()

		// Vul het bestand met de standaardconfiguratie
		configContent := `
neo4j:
  uri: "neo4j://localhost:7687"
  user: "neo4j"
  password: "neo4jpass"
`
		_, err = file.WriteString(configContent)
		if err != nil {
			log.Fatalf("Error writing to config file: %v", err)
		}
	} else if err != nil {
		log.Fatalf("Error checking config file: %v", err)
	} else {
		fmt.Println("Configuration file already exists at", configFile)
	}
}

func main() {
	// CLI parameter voor de file
	filePath := flag.String("f", "", "Path to the JSON file")
	flag.Parse()

	// Controleer of een bestand is opgegeven
	if *filePath == "" {
		log.Fatal("Usage: go run main.go -f <path to json file>")
	}

	// Open het opgegeven bestand
	file, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("Error opening JSON file: %v", err)
	}
	defer file.Close()

	// Verbinden met Neo4j
	uri := "neo4j://localhost:7687"
	user := "neo4j"
	password := "neo4jpass"

	driver, err := neo4j.NewDriver(uri, neo4j.BasicAuth(user, password, ""))
	if err != nil {
		log.Fatalf("Error connecting to Neo4j: %v", err)
	}
	defer driver.Close()

	// Maak een sessie zonder context
	session := driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()

	// Verwerk JSON regel voor regel
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var result HttpxResult
		if err := json.Unmarshal(scanner.Bytes(), &result); err != nil {
			log.Printf("Error parsing JSON: %v", err)
			continue
		}

		log.Printf("Processing URL: %s", result.URL)

		_, err := session.WriteTransaction(func(tx neo4j.Transaction) (any, error) {
			// Voeg Host toe
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

			// Voeg IP en relatie met Host toe
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

			// Voeg Tech nodes en relaties toe
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

			// Voeg ASN data toe
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
