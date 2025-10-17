package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
)

var debug *bool

const colorRed = "\033[0;31m"
const colorGreen = "\033[0;32m"
const colorNone = "\033[0m"

var friendlyLanguageAliases = map[string]string{
	"":           "English",
	"en":         "English",
	"eng":        "English",
	"english":    "English",
	"es":         "Spanish",
	"spa":        "Spanish",
	"spanish":    "Spanish",
	"fr":         "French",
	"fra":        "French",
	"fre":        "French",
	"french":     "French",
	"de":         "German",
	"deu":        "German",
	"ger":        "German",
	"german":     "German",
	"it":         "Italian",
	"ita":        "Italian",
	"italian":    "Italian",
	"pt":         "Portuguese",
	"por":        "Portuguese",
	"portuguese": "Portuguese",
	"pt-br":      "Brazilian Portuguese",
	"pt_br":      "Brazilian Portuguese",
	"ja":         "Japanese",
	"jpn":        "Japanese",
	"japanese":   "Japanese",
	"ko":         "Korean",
	"kor":        "Korean",
	"korean":     "Korean",
	"zh":         "Chinese",
	"zho":        "Chinese",
	"chi":        "Chinese",
	"zh-hans":    "Chinese (Simplified)",
	"zh_cn":      "Chinese (Simplified)",
	"zh-cn":      "Chinese (Simplified)",
	"zh_hans":    "Chinese (Simplified)",
	"zh-hant":    "Chinese (Traditional)",
	"zh_tw":      "Chinese (Traditional)",
	"zh-tw":      "Chinese (Traditional)",
	"zh_hant":    "Chinese (Traditional)",
	"ar":         "Arabic",
	"ara":        "Arabic",
	"arabic":     "Arabic",
	"bg":         "Bulgarian",
	"bul":        "Bulgarian",
	"bulgarian":  "Bulgarian",
	"ca":         "Catalan",
	"cat":        "Catalan",
	"catalan":    "Catalan",
	"cs":         "Czech",
	"ces":        "Czech",
	"cze":        "Czech",
	"czech":      "Czech",
	"da":         "Danish",
	"dan":        "Danish",
	"danish":     "Danish",
	"nl":         "Dutch",
	"nld":        "Dutch",
	"dut":        "Dutch",
	"dutch":      "Dutch",
	"et":         "Estonian",
	"est":        "Estonian",
	"estonian":   "Estonian",
	"fi":         "Finnish",
	"fin":        "Finnish",
	"finnish":    "Finnish",
	"el":         "Greek",
	"ell":        "Greek",
	"greek":      "Greek",
	"he":         "Hebrew",
	"heb":        "Hebrew",
	"hebrew":     "Hebrew",
	"hi":         "Hindi",
	"hin":        "Hindi",
	"hindi":      "Hindi",
	"hr":         "Croatian",
	"hrv":        "Croatian",
	"croatian":   "Croatian",
	"hu":         "Hungarian",
	"hun":        "Hungarian",
	"hungarian":  "Hungarian",
	"id":         "Indonesian",
	"ind":        "Indonesian",
	"indonesian": "Indonesian",
	"km":         "Khmer",
	"khm":        "Khmer",
	"khmer":      "Khmer",
	"lo":         "Lao",
	"lao":        "Lao",
	"laos":       "Lao",
	"lt":         "Lithuanian",
	"lit":        "Lithuanian",
	"lithuanian": "Lithuanian",
	"lv":         "Latvian",
	"lav":        "Latvian",
	"latvian":    "Latvian",
	"ms":         "Malay",
	"msa":        "Malay",
	"malay":      "Malay",
	"nb":         "Norwegian Bokmål",
	"nob":        "Norwegian Bokmål",
	"no":         "Norwegian",
	"nor":        "Norwegian",
	"norwegian":  "Norwegian",
	"pl":         "Polish",
	"pol":        "Polish",
	"polish":     "Polish",
	"ro":         "Romanian",
	"ron":        "Romanian",
	"rum":        "Romanian",
	"romanian":   "Romanian",
	"ru":         "Russian",
	"rus":        "Russian",
	"russian":    "Russian",
	"sk":         "Slovak",
	"slk":        "Slovak",
	"slovak":     "Slovak",
	"sl":         "Slovenian",
	"slv":        "Slovenian",
	"slovenian":  "Slovenian",
	"sv":         "Swedish",
	"swe":        "Swedish",
	"swedish":    "Swedish",
	"th":         "Thai",
	"tha":        "Thai",
	"thai":       "Thai",
	"tr":         "Turkish",
	"tur":        "Turkish",
	"turkish":    "Turkish",
	"uk":         "Ukrainian",
	"ukr":        "Ukrainian",
	"ukrainian":  "Ukrainian",
	"vi":         "Vietnamese",
	"vie":        "Vietnamese",
	"vietnamese": "Vietnamese",
}

func logInfo(message, content string) {
	if debug != nil && *debug {
		fmt.Println(colorGreen, "[Info]", colorNone, message+": ", content)
	}
}

func logError(message string, lines ...string) {
	fmt.Printf("%s[Error]%s %s\n", colorRed, colorNone, message)
	if len(lines) == 0 {
		return
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		fmt.Printf("  %s\n", trimmed)
	}
}

func requireAPIKey() (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY is not set. Set it to a valid OpenAI API key before running the translator.")
	}

	return apiKey, nil
}

func formatOpenAIError(err error) error {
	if err == nil {
		return nil
	}

	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.HTTPStatusCode {
		case 401:
			return fmt.Errorf("OpenAI rejected the request (401 Unauthorized). Verify that OPENAI_API_KEY is correct and has access to the selected model.")
		case 429:
			return fmt.Errorf("OpenAI rejected the request (429 Too Many Requests). You may have hit the rate limit or exceeded your quota; wait a bit or check your billing plan.")
		default:
			if apiErr.Message != "" {
				return fmt.Errorf("OpenAI API error (%d): %s", apiErr.HTTPStatusCode, apiErr.Message)
			}
			return fmt.Errorf("OpenAI API error (%d)", apiErr.HTTPStatusCode)
		}
	}

	msg := err.Error()
	if strings.Contains(msg, "status code: 401") {
		return fmt.Errorf("OpenAI rejected the request (401 Unauthorized). Verify that OPENAI_API_KEY is correct and has access to the selected model.")
	}
	if strings.Contains(msg, "status code: 429") {
		return fmt.Errorf("OpenAI rejected the request (429 Too Many Requests). You may have hit the rate limit or exceeded your quota; wait a bit or check your billing plan.")
	}

	return err
}

func friendlyLanguageName(code string) string {
	normalized := strings.ToLower(strings.TrimSpace(code))

	if friendly, ok := friendlyLanguageAliases[normalized]; ok {
		return friendly
	}

	return strings.TrimSpace(code)
}

func parseLanguages(raw string) []string {
	parts := strings.Split(raw, ",")
	languages := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			languages = append(languages, trimmed)
		}
	}
	return languages
}

func findStringsFileInDir(dirPath string) (string, error) {
	defaultFile := filepath.Join(dirPath, "Localizable.strings")
	if _, err := os.Stat(defaultFile); err == nil {
		return defaultFile, nil
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".strings" {
			return filepath.Join(dirPath, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("no .strings file found in %s", dirPath)
}

func candidatePathForLang(rootDir, lang string) (string, string, error) {
	normalized := strings.TrimSpace(strings.TrimSuffix(lang, ".lproj"))
	if normalized == "" {
		return "", "", fmt.Errorf("invalid base language provided")
	}

	candidateDirs := []string{normalized + ".lproj", normalized}
	for _, dirName := range candidateDirs {
		dirPath := filepath.Join(rootDir, dirName)
		info, err := os.Stat(dirPath)
		if err != nil || !info.IsDir() {
			continue
		}

		filePath, err := findStringsFileInDir(dirPath)
		if err == nil {
			return filePath, normalized, nil
		}
	}

	return "", "", fmt.Errorf("no localization file found for %q in %s", normalized, rootDir)
}

func locateBaseStringsFile(rootDir, baseLang string) (string, string, error) {
	if strings.TrimSpace(baseLang) != "" {
		path, normalized, err := candidatePathForLang(rootDir, baseLang)
		if err != nil {
			return "", "", err
		}
		return path, normalized, nil
	}

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return "", "", err
	}

	type candidate struct {
		path string
		lang string
	}

	candidates := make([]candidate, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".lproj") {
			continue
		}

		lang := strings.TrimSuffix(entry.Name(), ".lproj")
		path, normalized, err := candidatePathForLang(rootDir, lang)
		if err == nil {
			candidates = append(candidates, candidate{path: path, lang: normalized})
		}
	}

	if len(candidates) == 1 {
		return candidates[0].path, candidates[0].lang, nil
	}

	if len(candidates) > 1 {
		return "", "", fmt.Errorf("multiple localization directories found in %s; please specify -baselang", rootDir)
	}

	return "", "", fmt.Errorf("no .strings localization files found in %s", rootDir)
}

func resolveBasePath(fileArg, baseLangFlag string) (string, string, string, error) {
	if fileArg == "" {
		return "", "", "", fmt.Errorf("file path is required")
	}

	info, err := os.Stat(fileArg)
	if err != nil {
		return "", "", "", err
	}

	if !info.IsDir() {
		normalized := strings.TrimSpace(strings.TrimSuffix(baseLangFlag, ".lproj"))
		return fileArg, "", normalized, nil
	}

	basePath, normalized, err := locateBaseStringsFile(fileArg, baseLangFlag)
	if err != nil {
		return "", "", "", err
	}

	return basePath, fileArg, normalized, nil
}

func loadSourceData(path string) (map[string]interface{}, string, error) {
	ext := filepath.Ext(path)
	var (
		out map[string]interface{}
		err error
	)

	switch ext {
	case ".yaml", ".yml":
		out, err = openYAML(path)
	case ".json":
		out, err = openJSON(path)
	case ".strings":
		out, err = openStrings(path)
	default:
		err = fmt.Errorf("unsupported file extension: %s", ext)
	}

	if err != nil {
		return nil, "", err
	}

	return out, ext, nil
}

func buildOutputPath(language, explicitOutput, ext, iosRoot, baseFilePath string) string {
	if iosRoot != "" {
		return filepath.Join(iosRoot, language+".lproj", filepath.Base(baseFilePath))
	}

	if explicitOutput != "" {
		return explicitOutput
	}

	if ext == "" {
		ext = ".json"
	}

	return fmt.Sprintf("output-%s%s", language, ext)
}

func translateToLanguage(source map[string]interface{}, outputPath string, sourceLanguage string, targetLanguage string, model string, chunkSize int, force bool) error {
	flattenedData := flatten(source, "")

	isOutputPathExist := false
	if _, err := os.Stat(outputPath); err == nil {
		isOutputPathExist = true
	}

	allTranslated := make(map[string]string)
	existingOutput := make(map[string]interface{})
	if !force && isOutputPathExist {
		counter := 0
		ext := filepath.Ext(outputPath)
		var err error
		switch ext {
		case ".yaml", ".yml":
			existingOutput, err = openYAML(outputPath)
		case ".json":
			existingOutput, err = openJSON(outputPath)
		case ".strings":
			existingOutput, err = openStrings(outputPath)
		default:
			return fmt.Errorf("unsupported output file extension: %s", ext)
		}
		if err != nil {
			return err
		}
		alreadyTranslated := flatten(existingOutput, "")
		var keys []string
		for k := range alreadyTranslated {
			keys = append(keys, k)
		}
		for _, k := range keys {
			delete(flattenedData, k)
			allTranslated[k] = alreadyTranslated[k]
			counter++
		}
		logInfo("Skipping keys", strconv.Itoa(counter))
	} else if force {
		logInfo("Force", "enabled")
	}

	chunks := chunkKeys(flattenedData, chunkSize)

	duplicateKeyCount := make([]string, 0)
	progressCounter := 0
	totalChunks := len(chunks)
	logInfo("Keys to translate", strconv.Itoa(len(flattenedData)))
	fmt.Printf("\nTranslating %s into %s\n", sourceLanguage, targetLanguage)
	if totalChunks > 0 {
		fmt.Printf("This can take a few minutes b/c %v is slow", model)
		fmt.Printf("\rProgress: %d/%d\x1b[K", 0, totalChunks)
	} else {
		fmt.Println("Warning: No strings need translation. The destination file may already contain all keys or the source data might be empty.")
		fmt.Printf("\rProgress: %d/%d\x1b[K", 0, totalChunks)
	}
	for idx, chunk := range chunks {
		translatedChunk, err := translateString(chunk, sourceLanguage, targetLanguage, model)
		if err != nil {
			fmt.Printf("\rProgress: %d/%d\x1b[K\n", progressCounter, totalChunks)
			friendly := strings.TrimSpace(err.Error())
			lines := make([]string, 0, 3)
			if friendly != "" {
				lines = append(lines, friendly)
			}
			lines = append(lines,
				fmt.Sprintf("Chunk %d of %d failed while translating %s.", idx+1, totalChunks, targetLanguage),
				"The translator stopped before finishing this language. Please fix the issue and rerun.",
			)
			logError("translateString", lines...)
			return fmt.Errorf("translation halted after %d/%d chunks: %w", progressCounter, totalChunks, err)
		}
		for k, v := range translatedChunk {
			if _, ok := allTranslated[k]; !ok {
				allTranslated[k] = v
			} else {
				duplicateKeyCount = append(duplicateKeyCount, k)
			}

		}
		progressCounter += 1
		fmt.Printf("\rProgress: %d/%d\x1b[K", progressCounter, totalChunks)
	}
	unflatMap := unflattenJSON(allTranslated)
	var unSquished []byte
	outputExt := filepath.Ext(outputPath)
	switch outputExt {
	case ".yaml", ".yml":
		unSquished, _ = yaml.Marshal(unflatMap)
	case ".json":
		unSquished, _ = json.Marshal(unflatMap)
	case ".strings":
		unSquished, _ = toStrings(allTranslated)
	default:
		return fmt.Errorf("unsupported output file extension: %s", outputExt)
	}

	save(unSquished, outputPath)
	fmt.Println("\n\nSaved result in:", outputPath)
	return nil
}

func main() {
	filePath := flag.String("file", "", "Path to the source file or directory")
	languagesFlag := flag.String("lang", "", "Comma separated target languages")
	baseLangFlag := flag.String("baselang", "", "Base language identifier (used when -file points to a directory of .lproj folders)")
	baseLangAlias := flag.String("baselange", "", "Deprecated alias for -baselang")
	force := flag.Bool("force", false, "forces all strings to be translated")
	debug = flag.Bool("debug", false, "writes debug logs")
	outputPath := flag.String("output", "", "output path (only used when translating a single language)")
	model := flag.String("model", "gpt-4o-mini", "model")
	chunkSize := flag.Int("chunksize", 500, "number of letters per chunk")
	flag.Parse()

	languages := parseLanguages(*languagesFlag)
	if len(languages) == 0 {
		logError("language", "Please provide one or more languages using -lang")
		os.Exit(1)
	}

	baseLang := strings.TrimSpace(*baseLangFlag)
	if baseLang == "" {
		baseLang = strings.TrimSpace(*baseLangAlias)
	}

	baseFilePath, iosRoot, resolvedBaseLang, err := resolveBasePath(*filePath, baseLang)
	if err != nil {
		logError("filePath", err.Error())
		os.Exit(1)
	}

	if *outputPath != "" && len(languages) > 1 && iosRoot == "" {
		logError("output", "The -output flag can only be used when translating a single language")
		os.Exit(1)
	}

	if iosRoot != "" && *outputPath != "" {
		logInfo("output", "Ignoring -output because a localization directory was provided")
	}

	sourceData, ext, err := loadSourceData(baseFilePath)
	if err != nil {
		logError("open", err.Error())
		os.Exit(1)
	}

	sourceLanguageName := friendlyLanguageName(resolvedBaseLang)
	if sourceLanguageName == "" {
		sourceLanguageName = "English"
	}

	logInfo("File Path", baseFilePath)
	if iosRoot != "" {
		logInfo("Localization Root", iosRoot)
	}
	logInfo("Source Language", sourceLanguageName)
	logInfo("Languages", strings.Join(languages, ", "))

	for _, lang := range languages {
		output := buildOutputPath(lang, *outputPath, ext, iosRoot, baseFilePath)
		if err := translateToLanguage(sourceData, output, sourceLanguageName, lang, *model, *chunkSize, *force); err != nil {
			logError("translate", err.Error())
			os.Exit(1)
		}
	}
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

func translateString(chunk map[string]string, sourceLanguage string, targetLanguage string, model string) (map[string]string, error) {
	if len(chunk) == 0 {
		return nil, nil
	}
	input := chunkToString(chunk)
	params := chunkToParams(chunk)
	apiKey, err := requireAPIKey()
	if err != nil {
		return nil, err
	}
	client := openai.NewClient(apiKey)

	f := openai.FunctionDefinition{
		Name:        "upload",
		Description: "uploads the " + targetLanguage + " phrases translated from " + sourceLanguage,
		Parameters:  params,
	}
	t := openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &f,
	}

	dialogue := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: "You will be provided key value pair " + sourceLanguage + " phrases, and your task is to translate the " + sourceLanguage + " values into concise " + targetLanguage + " and upload them. The messages are for a localization for a mobile application. respond with json"},
		{Role: openai.ChatMessageRoleUser, Content: input},
	}

	resp, err := client.CreateChatCompletion(context.Background(),
		openai.ChatCompletionRequest{
			Model:    model,
			Messages: dialogue,
			Tools:    []openai.Tool{t},
		},
	)
	if err != nil {
		return nil, formatOpenAIError(err)
	}
	if len(resp.Choices) != 1 {
		return nil, fmt.Errorf("unexpected number of choices from OpenAI: %d", len(resp.Choices))
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
		logError("save", fmt.Sprintf("Error creating directory: %v", err))
		return
	}

	err := os.WriteFile(outputPath, json, 0644)
	if err != nil {
		logError("save", fmt.Sprintf("writing to file: %v", err))
	}
}

// openStrings reads a .strings file and converts it to a map[string]string
func openStrings(path string) (map[string]interface{}, error) {
	// Open the file
	file, err := os.Open(path)
	if err != nil {
		logError("openStrings", fmt.Sprintf("opening file: %v", err))
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
		logError("openStrings", fmt.Sprintf("opening reading file: %v", err))

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
	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

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
