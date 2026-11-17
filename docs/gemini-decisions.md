# Gemini Decisions

`google/gemini-decision-1` is a native classifier for closed-set judgments. It uses Google's [Decisions API](https://aistudio.google.com/docs/decisions-api), not Gemini text generation. In `/model`, filter by **judge** and assign Gemini Decision 1 to the `judge` role. It cannot be selected as the chat model. Codemode selects it automatically when it is the configured judge; scripts cannot choose a model or role.

## Use Gemini Decision 1 in codemode

`models.judge(context)` replaces `models.classify(model, context)` in codemode.
Assign the model to the `judge` role outside the script. The Go classifier APIs
remain available.

```js
const result = await models.judge({
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
	correct: result.answers.correct.value,
	correctProbability: result.answers.correct.classifier?.probability,
	action: result.answers.action.value,
	actionProbabilities: result.answers.action.classifier?.probabilities,
	quality: result.answers.quality.value,
};
```

The model registry records `models.judge` as a nested model call. Its usage and estimated cost join the codemode result.

## Configure Google authentication

Set `GEMINI_API_KEY` or configure a Google API key through the normal provider auth flow. The classifier sends the key in `x-goog-api-key`. It does not send a bearer token by default or put the key in the URL or request body.

The built-in model uses `https://generativelanguage.googleapis.com/v1beta/decisions`. A configured Google base URL override applies to the classifier. The model is available through the Gemini Developer API only. Google Vertex AI is not supported by this integration.

## Read the result

Each answer has a `type` and a scalar `value`: booleans are true when the native probability is at least 0.5, choices contain a selected key, and scores contain a number within the rubric. Native classifier models add a `classifier` object with available probabilities or confidence; chat-model answers do not fabricate this metadata. The result includes the chosen provider and model, combined usage for every attempted candidate, and a `stopReason`.

Gemini Decision 1 does not generate explanations. Check `stopReason` before reading answers. Errors and cancellation return no answers and set `stopReason` to `error` or `aborted`.

The catalog lists a 262,144-token context window, `$0.10` per million input tokens, and no output-token cost. Cost values are estimates from the catalog and can change.

## Request limits and supported input

The adapter accepts 1 to 1,000 questions. Question IDs must contain 1 to 64 ASCII letters, digits, underscores, or hyphens. Choice questions need 2 to 255 unique UTF-8 option keys. Each key permits 1 to 128 characters and at most 512 bytes. Internal ASCII spaces are allowed. Leading or trailing spaces, control characters, and other whitespace are not allowed. Score questions need 2 to 255 non-empty levels.

The model accepts JSON state and inline images. It serializes state as JSON text and sends each image as a base64 content block with its MIME type. The existing classifier context does not represent file URIs, audio, video, or PDF input, so this integration does not advertise those formats. The adapter does not estimate input tokens; requests that exceed the model context fail at the API boundary.
