# Gemini Decisions

`google/gemini-decision-1` is a native classifier for closed-set judgments. It uses Google's [Decisions API](https://aistudio.google.com/docs/decisions-api), not Gemini text generation. In `/model`, filter by **judge** and assign Gemini Decision 1 to a judge role. It cannot be selected as the chat model.

## Use Gemini Decision 1 in codemode

`models.getModelOfType` returns the model object that `models.classify` requires. Pass the object, not a `provider/id` string.

```js
const judge = await models.getModelOfType("classifier", "google", "gemini-decision-1");
if (!judge) throw new Error("Gemini Decision 1 is not registered");

const result = await models.classify(judge, {
	state: { answer: "4", expected: "4" },
	questions: {
		correct: {
			type: "bool",
			instructions: "Does the answer match the expected value?",
			criteria: { true: "The values match.", false: "The values differ." },
		},
		action: {
			type: "choice",
			instructions: "What should happen to this answer?",
			criteria: { accept: "The answer is correct.", review: "The answer needs review." },
		},
		quality: {
			type: "score",
			instructions: "Rate the answer against the expected value.",
			criteria: ["Incorrect", "Partly correct", "Correct"],
		},
	},
});

if (result.stopReason !== "stop") throw new Error(result.errorMessage);
return {
	correct: result.answers.correct.probability,
	action: result.answers.action.choice,
	actionProbabilities: result.answers.action.probabilities,
	quality: result.answers.quality.score,
};
```

The model registry records `models.classify` as a nested model call. Its usage and estimated cost join the codemode result.

## Configure Google authentication

Set `GEMINI_API_KEY` or configure a Google API key through the normal provider auth flow. The classifier sends the key in `x-goog-api-key`. It does not send a bearer token by default or put the key in the URL or request body.

The built-in model uses `https://generativelanguage.googleapis.com/v1beta/decisions`. A configured Google base URL override applies to the classifier. The model is available through the Gemini Developer API only. Google Vertex AI is not supported by this integration.

## Read the result

The result keeps the existing classifier types:

- A bool answer contains the probability that the answer is true.
- A choice answer contains the selected key, its confidence, and probabilities keyed by option. The probability slice returned to Go callers follows the criteria order in the request.
- A score answer contains the probability-weighted score and confidence. The result does not include per-level probabilities or the score legend.

Gemini Decision 1 does not generate explanations. Its bool confidence is not a separate result field. Check `stopReason` before reading answers. Errors and cancellation return no answers and set `stopReason` to `error` or `aborted`.

The catalog lists a 262,144-token context window, `$0.10` per million input tokens, and no output-token cost. Cost values are estimates from the catalog and can change.

## Request limits and supported input

The adapter accepts 1 to 1,000 questions. Question IDs must contain 1 to 64 ASCII letters, digits, underscores, or hyphens. Choice questions need 2 to 255 unique UTF-8 option keys. Each key permits 1 to 128 characters and at most 512 bytes. Internal ASCII spaces are allowed. Leading or trailing spaces, control characters, and other whitespace are not allowed. Score questions need 2 to 255 non-empty levels.

The model accepts JSON state and inline images. It serializes state as JSON text and sends each image as a base64 content block with its MIME type. The existing classifier context does not represent file URIs, audio, video, or PDF input, so this integration does not advertise those formats. The adapter does not estimate input tokens; requests that exceed the model context fail at the API boundary.
