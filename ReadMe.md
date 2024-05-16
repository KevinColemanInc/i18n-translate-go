# Translate json files with GPT-4/OpenAI

```
$ go install github.com/kevincolemaninc/i18n-translate-go@latest
$ i18n-translate-go -file "~/i18n/en.json" -lang "korean" -output ko.json -model gpt-4-turbo -chunksize 1000
```

or just pull and run the project

```
$ export OPENAI_API_KEY=...
$ go run main.go -file "~/projects/rn-app/i18n/en.json" -lang vi
```

output directory: `output-{language}.json`

## flags

| flag      | description                                                                                                                                                                                                                                                         |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| file      | path of the source english file                                                                                                                                                                                                                                     |
| lang      | language you want to translate to, (this is sent to gpt-4 and doesn't and shouldn't be a language abbreciation (vietnamese is better than vi)                                                                                                                       |
| output    | destination of the json file. default is `output-{lang}.json`                                                                                                                                                                                                       |
| model     | name of the completion model. default is gpt-4-turbo                                                                                                                                                                                                                |
| chunksize | To ensure accurate translation and prevent skipping phrases, limit the number of letters translated per request. For common languages, 2000 letters are suitable, while for less common languages like Lao, opt for 500 letters. The default limit is 2000 letters. |

## features / roadmap

- [x] concurrency (5 workers)
- [x] support multiple gpt models
- [x] support json (i18n js) and yaml (i18n rails)
- [ ] cache results (only update missing keys)
- [ ] automatically check for blank or missing translations
- [ ] retry blank or missing translations

## Example output

with 5 workers, chunksize of 1000, and 26,000 letters, this takes about 2 minutes

```
$ i18n-translate-go -file "./src/utils/i18n/en.json" -lang "korean" -output ko.json -model gpt-4-turbo -chunksize 1000

File Path: ./src/utils/i18n/en.json
Language: korean

This can take a few minutes b/c GPT4 is slow
Progress: 5/49
Saved result in: ko.json
```

### How does it work?

1. Flattens the json into a nested key structure: { "user": { "name": .. } } -> "user.name".
2. Chunks the key/values by length of the keys in characters [0].
3. Sends the chunks to completions API using function calling.
4. Unflattens the resulting json and saves it to disk.

The prompt is [here](https://github.com/KevinColemanInc/i18n-translate-go/blob/main/main.go#L151).

[0] - to minimize complexity and dependencies, I count characters instead of tokens

### How well does it work?

I had 2 native vietnamese speakers compare the automatic translation with the human translations. Both preferred the human translation, because the automated translation was too verbose and had tenses wrong. I updated the prompt to request it to "be concise."

## Known errors

> Translation error - you should restart this b/c the translations will not be complete.

If the chunkSize is too big and/or the language is in uncommon language (e.g. Lao), chatGPT doesn't translate all the strings you ask it.

> Failed to create completion as the model generated invalid Unicode output. Unfortunately, this can happen in rare situations. Consider reviewing your prompt or reducing the temperature of your request. You can retry your request, or contact us through our help center at help.openai.com if the error persists.

Re-run the script and consider reducing the chunksize