# Translate json files with GPT-4/OpenAI

```
go install https://github.com/kevincolemaninc/i18n-translate@latest 
```
or just pull and run the project

```
go run main.go -file "~/projects/rn-app/i18n/en.json" -lang vi
```

output directory: `output-{language}.json`

## flags

| flag      | description                                                                                                                                   |
|-----------|-----------------------------------------------------------------------------------------------------------------------------------------------|
| file      | path of the source english file                                                                                                               |
| lang      | language you want to translate to, (this is sent to gpt-4 and doesn't and shouldn't be a language abbreciation (vietnamese is better than vi) |
| output    | destination of the json file. default is `output-{lang}.json`                                                                                 |
| model     | name of the completion model. default is gpt-4-turbo                                                                                          |
| chunksize | To ensure accurate translation and prevent skipping phrases, limit the number of letters translated per request. For common languages, 2000 letters are suitable, while for less common languages like Lao, opt for 500 letters. The default limit is 2000 letters.

## features
- [x] concurrency (default 3)
- [x] multiple gpt models
- [ ] cache results (only update missing keys)
