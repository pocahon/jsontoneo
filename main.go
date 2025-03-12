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
	Host      string   `json:"host"`
	Status    int      `json:"status_code"`
	Words     int      `json:"words"`
	Lines     int      `json:"lines"`
	Resolvers []string `json:"resolvers"`
}

func main() {
	// CLI parameter for the file
	filePath := flag.String("f", "", "Path to the JSON file")
	flag.Parse()

	// Check if a file was provided
	if *filePath == "" {
		log.Fatal("Usage: go run main.go -f <path to JSON file>")
	}

	// Open the provided file
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

	// Create a session without context
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
                h.title = $title,
                h.scheme = $scheme,
                h.webserver = $webserver,
                h.status = $status,
                h.words = $words,
                h.lines = $lines
	    SET h.neo4j_label = $url
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

			// Add ASN data if available
			if result.ASN.ASNumber != "" {
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
