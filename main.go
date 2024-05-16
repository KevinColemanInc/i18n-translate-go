package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func main() {
	// Define flags for file path and language string
	filePath := flag.String("file", "", "Path to the JSON file")
	language := flag.String("lang", "", "Language string")
	outputPath := flag.String("output", "", "output path")
	model := flag.String("model", "gpt-4o", "model")
	chunkSize := flag.Int("chunksize", 500, "number of letters per chunk")
	flag.Parse()

	// Check if file path is provided
	if *filePath == "" {
		fmt.Println("Please provide a file path using -file flag")
		os.Exit(1)
	}

	// Check if language string is provided
	if *language == "" {
		fmt.Println("Please provide a language string using -lang flag")
		os.Exit(1)
	}

	if outputPath == nil || *outputPath == "" {
		*outputPath = "output-" + *language + ".json"
	}

	// Use the file path and language string here
	fmt.Println("File Path:", *filePath)
	fmt.Println("Language:", *language)
	fmt.Println("\nThis can take a few minutes b/c GPT4 is slow")
	var out map[string]interface{}
	var err error
	ext := filepath.Ext(*filePath)
	switch ext {
	case ".yaml", ".yml":
		out, err = openYAML(*filePath)
	case ".json":
		out, err = openJSON(*filePath)
	default:
		fmt.Errorf("unsupported file extension: %s", ext)
		return
	}

	if err != nil {
		fmt.Errorf(err.Error())
		return
	}
	flattenedData := flatten(out, "")
	chunks := chunkKeys(flattenedData, *chunkSize)

	allTranslated := make(map[string]string)
	duplicateKeyCount := make([]string, 0)
	locker := new(sync.Mutex)
	chunkChan := chunkGenerator(chunks)
	workerCount := 5
	var wg sync.WaitGroup
	workerPool := make(chan struct{}, workerCount)
	progressCounter := 0
	totalChunks := len(chunks)
	fmt.Printf("\rProgress: %d/%d\x1b[K", 0, totalChunks)
	for chunk := range chunkChan {
		wg.Add(1)
		go func(chunk map[string]string) {
			defer wg.Done()
			workerPool <- struct{}{}
			defer func() {
				<-workerPool
			}()
			translatedChunk, err := translateString(chunk, *language, *model)
			if err != nil {
				fmt.Printf(err.Error())
				fmt.Printf("Translation error - you should restart this b/c the translations will not be complete.\n")
				return
			}
			locker.Lock()
			for k, v := range translatedChunk {
				if _, ok := allTranslated[k]; !ok {
					allTranslated[k] = v
				} else {
					duplicateKeyCount = append(duplicateKeyCount, k)
				}

			}
			progressCounter += 1
			locker.Unlock()
			fmt.Printf("\rProgress: %d/%d\x1b[K", progressCounter, totalChunks)
		}(chunk)
	}
	wg.Wait()
	unflatMap := unflattenJSON(allTranslated)
	var unSquished []byte
	switch ext {
	case ".yaml", ".yml":
		unSquished, _ = yaml.Marshal(unflatMap)
	case ".json":
		unSquished, _ = json.Marshal(unflatMap)
	}

	save(unSquished, *outputPath)
	fmt.Println("Saved result in:", *outputPath)
}

func chunkGenerator(chunks []map[string]string) <-chan map[string]string {
	out := make(chan map[string]string)
	go func() {
		defer close(out)
		for _, chunk := range chunks {
			out <- chunk
		}
	}()
	return out
}

func chunkToString(chunk map[string]string) (stringChunk string) {
	for key, value := range chunk {
		stringChunk += key + ":" + value + "\n"
	}
	return
}

func chunkToParams(chunk map[string]string) jsonschema.Definition {
	properties := make(map[string]jsonschema.Definition)
	required := make([]string, 0, len(chunk))
	for key, value := range chunk {
		properties[key] = jsonschema.Definition{
			Type:        jsonschema.String,
			Description: value,
		}
		required = append(required, key)
	}

	return jsonschema.Definition{
		Type:       jsonschema.Object,
		Properties: properties,
		Required:   required,
	}
}

func translateString(chunk map[string]string, targetLanguage string, model string) (map[string]string, error) {
	input := chunkToString(chunk)
	params := chunkToParams(chunk)
	client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	f := openai.FunctionDefinition{
		Name:        "upload",
		Description: "uploads the " + targetLanguage + " phrases",
		Parameters:  params,
	}
	t := openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &f,
	}

	dialogue := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: "You will be provided key value pair English phrases, and your task is to translate the english values into concise " + targetLanguage + " and upload them. respond with json"},
		{Role: openai.ChatMessageRoleUser, Content: input},
	}

	resp, err := client.CreateChatCompletion(context.Background(),
		openai.ChatCompletionRequest{
			Model:    model,
			Messages: dialogue,
			Tools:    []openai.Tool{t},
		},
	)
	if err != nil || len(resp.Choices) != 1 {
		fmt.Printf("Completion error: err:%v len(choices):%v\n", err,
			len(resp.Choices))
		return nil, err
	}
	translatedChunk := make(map[string]string, len(chunk))
	for k, _ := range chunk {
		translatedChunk[k] = ""
	}

	totalCalls := 0
	foundUniqueKeys := 0
	unplannedKeys := make([]string, 0)
	for _, choice := range resp.Choices {
		msg := choice.Message
		for _, toolCall := range msg.ToolCalls {
			var params map[string]string
			json.Unmarshal([]byte(toolCall.Function.Arguments), &params)

			for k, v := range params {
				totalCalls += 1
				if chunkV, ok := translatedChunk[k]; ok && chunkV == "" {
					foundUniqueKeys += 1
					translatedChunk[k] = v
				} else if chunkV != "" {
					fmt.Println("unplanned key:", k)
					unplannedKeys = append(unplannedKeys, k)
				}
			}
		}
	}

	missingKeys := make([]string, 0)
	for k, v := range translatedChunk {
		if v == "" {
			fmt.Printf("missing value for key: %v. I recommend reducing the chunkSize and restarting the script.\n", k)
			missingKeys = append(missingKeys, k)
		}
	}

	return translatedChunk, nil //fmt.Errorf("no translation provided in response")
}

func unflattenJSON(flattened map[string]string) map[string]interface{} {
	nested := make(map[string]interface{})

	for key, value := range flattened {
		keys := strings.Split(key, ".")
		current := nested

		// Traverse the key path and create nested maps as needed
		for i := 0; i < len(keys)-1; i++ {
			if _, ok := current[keys[i]]; !ok {
				current[keys[i]] = make(map[string]interface{})
			}
			current = current[keys[i]].(map[string]interface{})
		}

		// Assign the value at the deepest level
		current[keys[len(keys)-1]] = value
	}

	return nested
}

func save(json []byte, outputPath string) {
	os.WriteFile(outputPath, json, 0644)
}

func openJSON(path string) (map[string]interface{}, error) {
	// Read the JSON content
	bytes, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return nil, err
	}

	// Unmarshal the JSON into a map[string]interface{}
	var data map[string]interface{}
	if err := json.Unmarshal(bytes, &data); err != nil {
		fmt.Println("Error unmarshaling JSON:", err)
		return nil, err
	}
	return data, nil
}

func chunkKeys(data map[string]string, chunkSize int) []map[string]string {
	chunks := make([]map[string]string, 0)
	currentChunk := make(map[string]string, 0)
	currentChunkSize := 0

	for key, value := range data {
		// Calculate the length of the current key and value
		keyValueLength := len(key) + len(value)

		// Check if adding the current key-value pair exceeds the chunk size
		if currentChunkSize+keyValueLength > chunkSize {
			// Append the current chunk to the chunks slice
			chunks = append(chunks, currentChunk)
			// Start a new chunk
			currentChunk = make(map[string]string, 0)
			currentChunkSize = 0
		}

		// Add the current key-value pair to the current chunk
		currentChunk[key] = value
		currentChunkSize += keyValueLength
	}

	// Append the last chunk to the chunks slice
	chunks = append(chunks, currentChunk)

	return chunks
}

func flatten(data map[string]interface{}, prefix string) map[string]string {
	flattened := make(map[string]string)

	for key, value := range data {
		// Create the full key path
		fullKey := fmt.Sprintf("%s.%s", prefix, key)
		if prefix == "" {
			fullKey = key
		}

		// If the value is a nested object, recursively flatten it
		if nested, ok := value.(map[string]interface{}); ok {
			nestedFlattened := flatten(nested, fullKey)
			for nestedKey, nestedValue := range nestedFlattened {
				flattened[nestedKey] = nestedValue
			}
		} else if nested, ok := value.(map[interface{}]interface{}); ok {
			stringNested := make(map[string]interface{})
			for k, v := range nested {
				stringNested[k.(string)] = v
			}
			nestedFlattened := flatten(stringNested, fullKey)
			for nestedKey, nestedValue := range nestedFlattened {
				flattened[nestedKey] = nestedValue
			}
		} else {
			// If the value is not an object, add it to the flattened map
			flattened[fullKey] = fmt.Sprintf("%v", value)
		}
	}

	return flattened
}

// openYAML loads a YAML file into the provided structure
func openYAML(filename string) (map[string]interface{}, error) {
	var translations map[string]interface{}
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	err = yaml.Unmarshal(data, &translations)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	return translations, nil
}
