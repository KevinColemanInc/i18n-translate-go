# Translate json files with GPT-4/OpenAI

```
$ go install github.com/kevincolemaninc/i18n-translate-go@latest
$ i18n-translate-go -file "~/i18n/en.json" -lang "korean" -output ko.json -model gpt-4o-mini -chunksize 1000
```

or just pull and run the project

```
$ export OPENAI_API_KEY=...
$ go run main.go -file "/Users/kevin/projects/avo-native/i18n/en.json" -lang vi
```

or for iOS Localized strings
```
$ go run main.go -chunksize 25 \
  -file "/Users/kevin/projects/698-expat-ios/698-expat-ios/Resources/Localizable" \
  -model gpt-4o \
  -baselang eng \
  -lang "lao,th,vi,ms"
```

The CLI will read the base language from `eng.lproj`, translate the phrases once per requested language, and save the results to matching `.lproj` folders (for example `lao.lproj/Localizable.strings`).

### Pre-PR checklist

Before opening a pull request with freshly generated strings, it helps to:

1. Run the translator with a conservative `-chunksize` the first time so it is easier to spot sections the model might skip.
2. Commit the source language file before generating output, then review the diff per locale to confirm no keys were lost or re-ordered unexpectedly.
3. Spot-check a handful of critical phrases (navigation labels, onboarding copy, etc.) with a fluent speaker or a second model pass to ensure context-sensitive wording looks natural.
4. If the project keeps multiple `.strings` tables, rerun the tool for each table individually (passing the exact file path via `-file`) so you can review smaller, focused diffs.
5. Make sure the `OPENAI_API_KEY` used to generate the copy is stored securely and rotated if it was shared for review.

### How the CLI discovers iOS strings files

When you pass a directory to `-file`, the tool looks for sub-directories that end in `.lproj` (the convention Xcode uses for localization bundles). It then selects a `.strings` file inside the folder to use as the base document:

1. If a `Localizable.strings` file exists it is always preferred. This mirrors the default file Xcode creates when you add a new strings table.
2. If there is no `Localizable.strings`, the first `.strings` file in the folder is used. This is a fall-back so that projects with custom table names (for example `Errors.strings`) still work.

Because most iOS projects keep `Localizable.strings` inside each language’s `.lproj` folder, the heuristic above usually points to the right file without additional configuration. If your project keeps multiple `.strings` files per language or uses a different naming convention, pass the exact base language directory with `-file` (for example `./Resources/en.lproj/Localizable.strings`) or provide `-baselang` so the CLI picks the correct folder. In short, for a “normal” iOS project that follows Apple’s defaults, the current discovery logic should find `Localizable.strings` automatically, but you can opt into a specific path whenever your structure differs.

output directory: `output-{language}{ext}`

## flags

| flag      | description |
|-----------|-------------|
| file      | Path of the source file (json/yaml/strings) or the localization directory that contains the `*.lproj` folders. |
| lang      | Comma separated languages you want to translate to. Use descriptive names (for example `vietnamese` instead of `vi`). |
| baselang  | Base language identifier used when `-file` points to a directory. This should match the `*.lproj` folder that contains the source strings (for example `en`, `eng`, or `Base`). |
| output    | Destination of the generated file. Only used when translating a single language. Defaults to `output-{lang}{ext}`. |
| model     | Name of the completion model. Default is `gpt-4o-mini`. |
| chunksize | To ensure accurate translation and prevent skipping phrases, limit the number of letters translated per request. For common languages, 2000 letters are suitable, while for less common languages like Lao, opt for 500 letters. The default limit is 2000 letters. |
| force     | Retranslate the file, ignoring any cached translations. |
| debug     | Print debug logs. |

## features / roadmap

- [x] concurrency (5 workers)
- [x] support multiple gpt models
- [x] support json (i18n js) and yaml (i18n rails)
- [x] support iOS Localization files
- [x] cache results (only update missing keys); enable "force" flag to retranslate everything
- [x] automatically check for blank or missing translations
- [ ] retry blank or missing translations

## Example output

with 5 workers, chunksize of 1000, and 26,000 letters, this takes about 2 minutes

```
$ i18n-translate-go -file "./src/utils/i18n/en.json" -lang "korean" -output ko.json -model gpt-4-turbo -chunksize 1000

File Path: ./src/utils/i18n/en.json
Language: korean

This can take a few minutes b/c model is slow
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

> Rate limit reached 

The number of concurrent requests is 3. This should be fine for most use-cases, but if it isn't the value is hard coded.
