<img src="https://r2cdn.perplexity.ai/pplx-full-logo-primary-dark%402x.png" class="logo" width="120"/>

## **Direct Answer: Filename Specification in API Responses**

**No, there is currently no simple, built-in way to specify output filenames directly in the JSON schemas** for OpenAI, Perplexity, or Ollama structured outputs. However, here are the straightforward approaches available:

### **The Simple Solution: Include Filename in Your JSON Schema**

The most direct approach is to simply include a **`filename` field** in your JSON schema response structure. Here's how each platform handles this:

### **OpenAI - Structured Output with Filename Field**

OpenAI's structured outputs support filename specification through standard JSON schema properties[^1][^2]:

```json
{
  "type": "json_schema",
  "json_schema": {
    "name": "file_output",
    "schema": {
      "type": "object",
      "properties": {
        "filename": {
          "type": "string",
          "description": "The exact filename to use when saving this file"
        },
        "content": {
          "type": "string",
          "description": "The file content"
        },
        "file_type": {
          "type": "string",
          "description": "The type of file generated"
        }
      },
      "required": ["filename", "content"]
    },
    "strict": true
  }
}
```

This approach works reliably with OpenAI's **100% guaranteed structured output compliance**[^2].

### **Perplexity - JSON Schema with Filename**

Perplexity supports structured outputs through JSON schema[^3][^4] where you can specify filename fields:

```json
{
  "type": "json_schema", 
  "json_schema": {
    "schema": {
      "type": "object",
      "properties": {
        "output_filename": {
          "type": "string",
          "description": "Desired filename for the generated file"
        },
        "file_content": {
          "type": "string"
        }
      },
      "required": ["output_filename", "file_content"]
    }
  }
}
```


### **Ollama - Format Parameter with Filename**

Ollama's new structured outputs (version 0.5+) use the `format` parameter[^5][^6]:

```json
{
  "format": {
    "type": "object",
    "properties": {
      "filename": {
        "type": "string",
        "description": "The name of the output file"
      },
      "file_data": {
        "type": "string"
      }
    },
    "required": ["filename", "file_data"]
  }
}
```


### **Key Limitations Across All Platforms**

1. **No Special JSON Schema Keywords**: There are no specific JSON Schema keywords like `filename`, `outputName`, or similar that have special meaning for file output[^1][^7].
2. **Standard Property Fields Only**: All platforms treat filename specifications as regular string properties in the schema - there's no special handling.
3. **No Direct File I/O**: None of these APIs actually handle file upload/download directly - they work with text/JSON responses that you then save as files[^8][^9].

### **Why This Approach Works**

The filename-in-schema approach is simple and effective because:

- **Universal Support**: All three platforms support basic string fields in JSON schemas
- **Reliable Parsing**: You get the exact filename back in a predictable JSON structure
- **No Complex Logic**: No need for regex patterns or complex prompt engineering
- **Type Safety**: The filename comes back as a structured field, not embedded in text


### **Simple Go Example**

```go
type FileOutput struct {
    Filename    string `json:"filename"`
    Content     string `json:"content"`
    ContentType string `json:"content_type"`
}

// In your JSON schema:
schema := map[string]interface{}{
    "type": "object",
    "properties": map[string]interface{}{
        "filename": map[string]interface{}{
            "type": "string",
            "description": "Exact filename to use",
        },
        "content": map[string]interface{}{
            "type": "string",
        },
    },
    "required": []string{"filename", "content"},
}
```


### **Recommendation**

**Use the filename field approach** instead of the complex solutions from previous examples. It's:

- **Simpler to implement**
- **Works across all three platforms**
- **Reliable and predictable**
- **Easier to maintain and debug**

The regex approach you mentioned stopped working in Perplexity[^10] precisely because it was fragile - structured JSON schemas with explicit filename fields are much more robust and portable across different AI providers.

<div style="text-align: center">⁂</div>

[^1]: https://platform.openai.com/docs/guides/structured-outputs

[^2]: https://openai.com/index/introducing-structured-outputs-in-the-api/

[^3]: https://docs.perplexity.ai/guides/structured-outputs

[^4]: https://python.useinstructor.com/integrations/perplexity/

[^5]: https://ollama.com/blog/structured-outputs

[^6]: https://ollama.com/blog/structured-outputs?__from__=talkingdev

[^7]: https://platform.openai.com/docs/guides/structured-outputs/introduction

[^8]: https://community.openai.com/t/filenames-in-code-interpreters-assistant-api/989609

[^9]: https://community.openai.com/t/how-to-specify-file-name-when-uploading-a-file-with-python-api/517789

[^10]: https://www.reddit.com/r/perplexity_ai/comments/1hhhwhl/can_i_have_the_api_return_in_json_and_only_json/

[^11]: https://www.andela.com/blog-posts/openai-structured-outputs-how-to-guide-for-developers

[^12]: https://pkg.go.dev/github.com/rsaranusc/openai-compatible/jsonschema

[^13]: https://www.youtube.com/watch?v=0lirLO0Nfl4

[^14]: https://fossies.org/linux/openai-python/src/openai/types/shared_params/response_format_json_schema.py

[^15]: https://developer.mamezou-tech.com/en/blogs/2024/08/10/openai-structured-output-intro/

[^16]: https://devblogs.microsoft.com/semantic-kernel/using-json-schema-for-structured-output-in-net-for-openai-models/

[^17]: https://platform.openai.com/docs/api-reference/files

[^18]: https://simmering.dev/blog/openai_structured_output/

[^19]: https://www.youtube.com/watch?v=EDBSNUhNe2Q

[^20]: https://github.com/nickolu/gpt-file-renamer

[^21]: https://simonwillison.net/2024/Aug/6/openai-structured-outputs/

[^22]: https://dev.to/yigit-konur/the-art-of-the-description-your-ultimate-guide-to-optimizing-llm-json-outputs-with-json-schema-jne

[^23]: https://community.openai.com/t/uploaded-file-name-becomes-file-unconditionally/16867

[^24]: https://gist.github.com/Donavan/9be122ff9d471c07da7bc74bab1d49ee

[^25]: https://deno.land/x/openai@v4.40.2/resources/files.ts?s=FileObject

[^26]: https://developers.oxylabs.io/scraping-solutions/web-scraper-api/targets/perplexity

[^27]: https://www.youtube.com/watch?v=LOe2FMuBpT8

[^28]: https://search.r-project.org/CRAN/refmans/perplexR/html/responseParser.html

[^29]: https://docs.perplexity.ai/guides/search-domain-filters

[^30]: https://gist.github.com/philsturgeon/e11b4cd603666b54d6436de6542998b7

[^31]: https://rdrr.io/cran/perplexR/man/responseParser.html

[^32]: https://docs.perplexity.ai/llms-full.txt

[^33]: https://brandur.org/elegant-apis

[^34]: https://search.r-project.org/CRAN/refmans/gptstudio/html/query_api_perplexity.html

[^35]: https://www.reddit.com/r/perplexity_ai/comments/1gjfov3/structured_json_output_on_perplexity_api/

[^36]: https://stackoverflow.com/questions/79396666/whats-the-reccommended-approach-for-building-a-json-schema-file-for-complex-typ

[^37]: https://rdrr.io/cran/gptstudio/man/query_api_perplexity.html

[^38]: https://docs.perplexity.ai/guides/getting-started

[^39]: https://community.n8n.io/t/using-perplexity-api-with-the-ai-tools-agent/54308

[^40]: https://community.openai.com/t/trying-to-use-structured-output-as-part-of-payload-for-an-api-request/904074

[^41]: https://github.com/567-labs/instructor/issues/1005

[^42]: https://pub.dev/documentation/langchain_ollama/latest/langchain_ollama/OllamaResponseFormat.html

[^43]: https://ollama.com/coolhand/schollama:14b

[^44]: https://www.youtube.com/watch?v=ljQ0i-F34a4

[^45]: https://stackoverflow.com/questions/78421795/is-there-a-simpler-way-of-using-file-names-as-a-value-in-a-json-file-using-a-sch

[^46]: https://towardsdatascience.com/structured-llm-output-using-ollama-73422889c7ad/

[^47]: https://www.reddit.com/r/ollama/comments/1jflnxl/structured_outputs_in_ollama_whats_your_recipe/

[^48]: https://ollama.com/coolhand/filellama

[^49]: https://ollama.com/coolhand/filellama:1b

[^50]: https://www.tamingllms.com/notebooks/structured_output.html

[^51]: https://stackoverflow.com/questions/70113545/json-schema-take-values-from-another-file-non-json-take-file-names

[^52]: https://github.com/jmorganca/ollama/issues/1710

[^53]: https://ollama.com/coolhand/schollama:24b

[^54]: https://elegantcode.com/2024/12/13/6998/

[^55]: https://www.reddit.com/r/ollama/comments/1h1tt3v/how_to_format_the_response_the_model_gives/

[^56]: https://www.youtube.com/watch?v=KXQU3mJTvuw

[^57]: https://geshan.com.np/blog/2025/02/ollama-api/

[^58]: https://ollama.com/coolhand/filellama:12b/blobs/d1e769600ecf

[^59]: https://www.youtube.com/watch?v=BgJNYT8voO4

