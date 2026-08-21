package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeYunwuOpenAIImagesRequest_GPTImageCompatibility(t *testing.T) {
	body, err := normalizeYunwuOpenAIImagesRequest([]byte(`{"model":"gpt-image-1","prompt":"draw a cat","n":1,"response_format":"url"}`), "application/json")
	require.NoError(t, err)
	require.Equal(t, "gpt-image-1", gjson.GetBytes(body, "model").String())
	require.Equal(t, "draw a cat", gjson.GetBytes(body, "prompt").String())
	require.Equal(t, int64(1), gjson.GetBytes(body, "n").Int())
	require.False(t, gjson.GetBytes(body, "response_format").Exists())
}

func TestNormalizeYunwuOpenAIImagesRequest_KeepsOtherModelOptions(t *testing.T) {
	body, err := normalizeYunwuOpenAIImagesRequest([]byte(`{"model":"imagen-3","prompt":"draw a cat","response_format":"url"}`), "application/json")
	require.NoError(t, err)
	require.Equal(t, "url", gjson.GetBytes(body, "response_format").String())
}

func TestNormalizeYunwuOpenAIImagesRequest_DefaultsGPTImageModel(t *testing.T) {
	body, err := normalizeYunwuOpenAIImagesRequest([]byte(`{"prompt":"draw a cat","response_format":"b64_json"}`), "application/json")
	require.NoError(t, err)
	require.Equal(t, "gpt-image-1", gjson.GetBytes(body, "model").String())
	require.False(t, gjson.GetBytes(body, "response_format").Exists())
}

func TestNormalizeYunwuOpenAIImagesRequest_RejectsMissingPrompt(t *testing.T) {
	_, err := normalizeYunwuOpenAIImagesRequest([]byte(`{"model":"gpt-image-1"}`), "application/json")
	require.EqualError(t, err, "prompt is required")
}

func TestNormalizeYunwuOpenAIImagesRequest_PreservesMultipartBody(t *testing.T) {
	body := []byte("multipart image bytes")
	got, err := normalizeYunwuOpenAIImagesRequest(body, "multipart/form-data; boundary=test")
	require.NoError(t, err)
	require.Equal(t, body, got)
}

func TestBuildYunwuGeminiImageGenerationRequest(t *testing.T) {
	body, model, ok, err := buildYunwuGeminiImageGenerationRequest([]byte(`{"model":"gemini-2.5-flash-image","prompt":"draw a cat","size":"1024x1024"}`), "application/json", "/v1/images/generations")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "gemini-2.5-flash-image", model)
	require.Equal(t, "draw a cat", gjson.GetBytes(body, "contents.0.parts.0.text").String())
	require.Equal(t, "IMAGE", gjson.GetBytes(body, "generationConfig.responseModalities.0").String())
	require.Equal(t, "1K", gjson.GetBytes(body, "generationConfig.imageConfig.imageSize").String())
}

func TestBuildYunwuGeminiImageGenerationRequestRejectsMultipleImages(t *testing.T) {
	_, _, ok, err := buildYunwuGeminiImageGenerationRequest([]byte(`{"model":"gemini-3.1-flash-image","prompt":"draw a cat","n":2}`), "application/json", "/v1/images/generations")
	require.True(t, ok)
	require.EqualError(t, err, "Gemini image generation supports n=1")
}

func TestOpenAIImagesResponseFromGemini(t *testing.T) {
	response, err := openAIImagesResponseFromGemini([]byte(`{"candidates":[{"content":{"parts":[{"text":"done"},{"inlineData":{"mimeType":"image/png","data":"UE5H"}}]}}]}`))
	require.NoError(t, err)
	require.Equal(t, "UE5H", response["data"].([]map[string]any)[0]["b64_json"])
}
