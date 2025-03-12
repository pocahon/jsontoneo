package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

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
	Host      string   `json:"host"` // Ensure that this field contains an IP address
	Status    int      `json:"status_code"`
	Words     int      `json:"words"`
	Lines     int      `json:"lines"`
	Resolvers []string `json:"resolvers"`
}

func main() {
	// CLI parameter for the JSON file (expects JSON Lines format)
	filePath := flag.String("f", "", "Path to the JSON file (JSON Lines format expected)")
	flag.Parse()

	if *filePath == "" {
		log.Fatal("Usage: go run main.go -f <path to JSON file>")
	}

	// Open the JSON file
	file, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("Error opening JSON file: %v", err)
	}
	defer file.Close()

	// Connect to Neo4j
	uri := "neo4j://localhost:7687"
	user := "neo4j"
	password := "neo4jpass"

	driver, err := neo4j.NewDriver(uri, neo4j.BasicAuth(user, password, ""))
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
			// Create or update the Host node (and store the node)
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

			// Add the IP node and create the relationship with the Host
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

			// Add Tech nodes and create the relationships
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

			// Add ASN data if available
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
