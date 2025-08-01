Below is one suggested “to‑do” list for converting from our current
regex–based file extraction (which looks for code fences with headers
like “File: …”) to structured I/O. In addition to the files already
mentioned above in the discussion, you’ll also need to update the
following:

 • in the gateway package (core/gateway.go), update the way that the
prompt is built and the way the AI response is interpreted. Instead of
instructing the LLM to “Return complete files” that are later
extracted with a regular expression, the gateway should send a system
prompt that instructs the LLM to produce a structured result (for
example, as valid JSON or XML). The code that passes messages to the
provider (whether OpenAI or Perplexity) must be adjusted so that the
downstream “extract” functions can simply parse out the JSON object
(or XML tree) containing the file name, language, and content fields.
This may include adding a new helper such as “ExtractStructuredFiles”
that does a json.Unmarshal rather than applying a regex.

 • in openai/openai.go, modify the CompleteChat function so that it
passes along a system instruction (or include a specially formatted
prompt segment) telling the LLM, for example, “Return your output as a
JSON object with a key ‘files’ whose value is an array of objects.
Each object should have exactly the keys ‘name’, ‘language’, and
‘content’.” In other words, remove our old assumption that the LLM
returns text with code fences and let the provider know that the
output is now structured. (You should take care not to “invent” any
new library functions; instead, simply have the function return the
raw response text for further processing via Go’s json package.) 

 • in perplexity/perplexity.go, similarly change the client’s
CompleteChat method so that—instead of relying on regex extraction—the
client (or later the caller via a common “gateway”) expects and parses
a structured output. That may mean, for example, adding logic after
unmarshalling Perplexity’s own response to verify that the text is
valid JSON (or XML) before returning it.

 • in core/chat.go (and the accompanying cli/cli.go and cli_test.go),
update the functions OutfilesRegex and ExtractFiles. Rather than
building and applying regular expressions keyed off a “File:” header
and code fences, you should remove or deprecate these functions and
instead call the new structured extraction routine (for example, one
that does a json.Unmarshal of the response text into a slice of a
FileLang struct). (Later tests will also have to be updated to expect
structured output instead of the regex‐matched text.) 

 • finally, update aidda as well. Since the AIDDA module uses both
file extraction (to read and commit files) and LLM instructions (to
generate editing commands), update its behavior so that when code is
to be generated or extracted it calls the new structured I/O routines.
This means its prompt–generation and file–extraction logic (for
example, in aidda.go and the tests in aidda/aidda_test.go) must be
altered to expect JSON (or whichever structured format you choose). 

In short, the bulk of the work is to remove our “extract by regex”
approach (for example, the OutfilesRegex function in core/chat.go and
its use in cli/cli.go) and replace it by enforcing a strict output
format from the LLM. In gateway.go you then merge that with the
provider–specific implementations in openai/openai.go and
perplexity/perplexity.go so that both providers return structured
output that your Grokker code then parses (using json.Unmarshal or an
XML parser). Finally, update any unit tests (e.g. in cli_test.go and
aidda) to match the new output format.

These modifications—in conjunction with updating system prompts
throughout the code to instruct the LLM “Return structured JSON (or
XML)”—will allow us to switch to a robust, structured file I/O
mechanism that does not depend on fragile regexes.
