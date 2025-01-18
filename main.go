package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
	"gopkg.in/yaml.v2"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var debug *bool

const colorRed = "\033[0;31m"
const colorGreen = "\033[0;32m"
const colorNone = "\033[0m"

func logInfo(message, content string) {
	if debug != nil && *debug {
		fmt.Println(colorGreen, "[Info]", colorNone, message+": ", content)
	}
}

func logError(message, content string) {
	fmt.Println(colorRed, "[Error]", colorNone, message+": ", content)
}

func main() {
	// Define flags for file path and language string
	filePath := flag.String("file", "", "Path to the *.json or *.Localizable strings file")
	language := flag.String("lang", "", "Language string")
	force := flag.Bool("force", false, "forces all strings to be translated")
	debug = flag.Bool("debug", false, "writes debug logs")
	outputPath := flag.String("output", "", "output path")
	model := flag.String("model", "gpt-4o-mini", "model")
	chunkSize := flag.Int("chunksize", 500, "number of letters per chunk")
	flag.Parse()

	// Check if file path is provided
	if *filePath == "" {
		logError("filePath", "Please provide a file path using -file flag")
		os.Exit(1)
	}

	// Check if language string is provided
	if *language == "" {
		logError("language", "Please provide a language string using -lang flag")
		os.Exit(1)
	}

	if outputPath == nil || *outputPath == "" {
		*outputPath = "output-" + *language + ".json"
	}

	// Use the file path and language string here
	logInfo("File Path", *filePath)
	logInfo("Language", *language)
	var out map[string]interface{}
	var err error
	ext := filepath.Ext(*filePath)
	switch ext {
	case ".yaml", ".yml":
		out, err = openYAML(*filePath)
	case ".json":
		out, err = openJSON(*filePath)
	case ".strings":
		out, err = openStrings(*filePath)
	default:
		logError("filePath", fmt.Errorf("unsupported file extension: %s", ext).Error())
		return
	}

	if err != nil {
		logError("open", err.Error())
		return
	}
	flattenedData := flatten(out, "")
	// inspect the output file for already translated keys

	isOutputPathExist := false
	if _, err := os.Stat(*outputPath); err == nil {
		isOutputPathExist = true
	}
	allTranslated := make(map[string]string)
	existingOutput := make(map[string]interface{})
	if !(*force) && isOutputPathExist {
		counter := 0
		ext := filepath.Ext(*outputPath)
		switch ext {
		case ".yaml", ".yml":
			existingOutput, err = openYAML(*outputPath)
		case ".json":
			existingOutput, err = openJSON(*outputPath)
		case ".strings":
			existingOutput, err = openStrings(*outputPath)
		default:
			err := fmt.Errorf("unsupported file extension: %s", ext)
			logError("open", err.Error())
			return
		}
		alreadyTranslated := flatten(existingOutput, "")
		var keys []string
		for k := range alreadyTranslated {
			keys = append(keys, k)
		}
		for _, k := range keys {
			counter += 1
			delete(flattenedData, k)
			allTranslated[k] = alreadyTranslated[k]
		}
		logInfo("Skipping keys", strconv.Itoa(counter))
	} else {
		logInfo("Force", "enabled")
	}

	chunks := chunkKeys(flattenedData, *chunkSize)

	duplicateKeyCount := make([]string, 0)
	locker := new(sync.Mutex)
	chunkChan := chunkGenerator(chunks)
	workerCount := 2
	var wg sync.WaitGroup
	workerPool := make(chan struct{}, workerCount)
	progressCounter := 0
	totalChunks := len(chunks)
	logInfo("Keys to translate", strconv.Itoa(len(flattenedData)))
	fmt.Printf("\nThis can take a few minutes b/c %v is slow", *model)
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
				logError("translateString", err.Error()+"\n You should restart this b/c the translations will not be complete.")
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
	case ".strings":
		unSquished, _ = toStrings(allTranslated)
	}

	save(unSquished, *outputPath)
	fmt.Println("\n\nSaved result in:", *outputPath)
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
	if len(chunk) == 0 {
		return nil, nil
	}
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
		{Role: openai.ChatMessageRoleSystem, Content: "You will be provided key value pair English phrases, and your task is to translate the english values into concise " + targetLanguage + " and upload them. The messages are for an localization for a mobile application. respond with json"},
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
			err := json.Unmarshal([]byte(toolCall.Function.Arguments), &params)
			if err != nil {
				logError("translateString", err.Error())
			}

			for k, v := range params {
				totalCalls += 1
				if chunkV, ok := translatedChunk[k]; ok && chunkV == "" {
					foundUniqueKeys += 1
					translatedChunk[k] = v
				} else if chunkV != "" {
					logError("unexpected key from llm", "The LLM returned a translation that was not expected. You may need to re-run this.")
					unplannedKeys = append(unplannedKeys, k)
				}
			}
		}
	}

	missingKeys := make([]string, 0)
	for k, v := range translatedChunk {
		if v == "" {
			logError("missing translations", fmt.Sprintf("missing value for key: %v. Reduce the chunkSize and restarting the script.", k))
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

	// Create the directory if it doesn't exist
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		logError("save", fmt.Sprintf("Error creating directory: %v\n", err))
		return
	}

	err := os.WriteFile(outputPath, json, 0644)
	if err != nil {
		logError("save", fmt.Sprintf("writing to file: %v\n", err))
	}
}

// openStrings reads a .strings file and converts it to a map[string]string
func openStrings(path string) (map[string]interface{}, error) {
	// Open the file
	file, err := os.Open(path)
	if err != nil {
		logError("openStrings", fmt.Sprintf("opening file: %v\n", err))
		return nil, err
	}
	defer file.Close()

	// Initialize the map to store the key-value pairs
	data := make(map[string]interface{})

	// Regular expressions to match key-value pairs and comments
	reKeyValue := regexp.MustCompile(`^\s*"(.*?)"\s*=\s*"(.*?)"\s*;`)
	reComment := regexp.MustCompile(`^\s*(//|/\*|\*|--).*`)

	// Scanner to read the file line by line
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments
		if reComment.MatchString(line) {
			continue
		}

		// Match key-value pairs
		matches := reKeyValue.FindStringSubmatch(line)
		if len(matches) == 3 {
			key := matches[1]
			value := matches[2]
			data[key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		logError("openStrings", fmt.Sprintf("opening reading file: %v\n", err))

		return nil, err
	}

	return data, nil
}
func openJSON(path string) (map[string]interface{}, error) {
	// Read the JSON content
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Unmarshal the JSON into a map[string]interface{}
	var data map[string]interface{}
	if err := json.Unmarshal(bytes, &data); err != nil {
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

// toStrings converts the data object to a strings file type byte array
func toStrings(data map[string]string) ([]byte, error) {
	// Create a buffer to hold the output
	var buffer bytes.Buffer

	// Sort the keys for consistent output
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Write each key-value pair to the buffer
	for _, key := range keys {
		value := data[key]
		_, err := buffer.WriteString(fmt.Sprintf("\"%s\" = \"%s\";\n", key, value))
		if err != nil {
			return nil, fmt.Errorf("error writing to buffer: %v", err)
		}
	}

	return buffer.Bytes(), nil
}

func toStringMap(m map[string]interface{}) map[string]string {
	stringMap := make(map[string]string)
	for k, v := range m {
		if sv, ok := v.(string); !ok {
			return stringMap
		} else {
			stringMap[k] = sv
		}
	}
	return stringMap
}

func flatten(data map[string]interface{}, prefix string) map[string]string {
	stringMap := toStringMap(data)

	if len(stringMap) == len(data) {
		return stringMap
	}

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
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	err = yaml.Unmarshal(data, &translations)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	return translations, nil
}
