import http from 'node:http';

const port = Number(process.env.JOYBUILD_PROXY_PORT || 18081);
const host = process.env.JOYBUILD_PROXY_HOST || '127.0.0.1';
// Accept either the host root or a value ending in /v1; the native endpoint is
// always appended exactly once below.
const baseUrl = (process.env.JOYBUILD_BASE_URL || 'http://ai-api.jdcloud.com')
  .replace(/\/+$/, '')
  .replace(/\/v1$/, '');
const apiKey = process.env.JOYBUILD_API_KEY;
const defaultModel = process.env.JOYBUILD_MODEL || 'Gemini-3.1-Flash-Lite';

if (!apiKey) {
  console.error('JOYBUILD_API_KEY is required');
  process.exit(1);
}

function readJson(req) {
  return new Promise((resolve, reject) => {
    let body = '';
    req.setEncoding('utf8');
    req.on('data', chunk => {
      body += chunk;
    });
    req.on('end', () => {
      try {
        resolve(body ? JSON.parse(body) : {});
      } catch (err) {
        reject(err);
      }
    });
    req.on('error', reject);
  });
}

function normalizeRole(role) {
  return role === 'assistant' ? 'model' : 'user';
}

function dataUrlToInlineData(url) {
  const match = /^data:([^;,]+);base64,(.*)$/s.exec(url);
  if (!match) {
    return undefined;
  }
  return {
    mimeType: match[1],
    data: match[2],
  };
}

function openAIContentToParts(content) {
  if (typeof content === 'string') {
    return content ? [{ text: content }] : [];
  }
  if (!Array.isArray(content)) {
    return [];
  }
  const parts = [];
  for (const item of content) {
    if (!item || typeof item !== 'object') {
      continue;
    }
    if (item.type === 'text' && item.text) {
      parts.push({ text: item.text });
      continue;
    }
    if (item.type === 'image_url' && item.image_url?.url) {
      const url = item.image_url.url;
      const inlineData = dataUrlToInlineData(url);
      if (inlineData) {
        parts.push({ inlineData });
      } else {
        parts.push({ fileData: { fileUri: url } });
      }
    }
  }
  return parts;
}

function buildJoyBuildRequest(openAIReq) {
  const contents = [];
  const systemParts = [];
  for (const message of openAIReq.messages || []) {
    const parts = openAIContentToParts(message.content);
    if (!parts.length) {
      continue;
    }
    if (message.role === 'system') {
      systemParts.push(...parts);
      continue;
    }
    contents.push({
      role: normalizeRole(message.role),
      parts,
    });
  }
  if (!contents.length && systemParts.length) {
    contents.push({
      role: 'user',
      parts: [{ text: 'Please follow the system instruction.' }],
    });
  }
  const generationConfig = {};
  if (openAIReq.max_tokens !== undefined) {
    generationConfig.maxOutputTokens = openAIReq.max_tokens;
  }
  if (openAIReq.temperature !== undefined) {
    generationConfig.temperature = openAIReq.temperature;
  }
  if (openAIReq.top_p !== undefined) {
    generationConfig.topP = openAIReq.top_p;
  }
  if (Array.isArray(openAIReq.stop)) {
    generationConfig.stopSequences = openAIReq.stop;
  }
  if (openAIReq.response_format?.type === 'json_object') {
    generationConfig.responseMimeType = 'application/json';
  }
  return {
    model: openAIReq.model || defaultModel,
    contents,
    ...(systemParts.length ? { systemInstruction: { parts: systemParts } } : {}),
    ...(Object.keys(generationConfig).length ? { generationConfig } : {}),
  };
}

function collectPartText(value) {
  if (!value) {
    return '';
  }
  if (typeof value === 'string') {
    return value;
  }
  if (Array.isArray(value)) {
    return value.map(collectPartText).join('');
  }
  if (typeof value === 'object') {
    if (typeof value.text === 'string') {
      return value.text;
    }
    return collectPartText(value.content) || collectPartText(value.parts);
  }
  return '';
}

function extractJoyBuildText(data) {
  for (const key of ['output_text', 'text', 'content', 'result', 'response']) {
    if (typeof data?.[key] === 'string' && data[key]) {
      return data[key];
    }
  }
  if (Array.isArray(data?.candidates)) {
    return data.candidates.map(candidate => collectPartText(candidate?.content?.parts)).join('');
  }
  return collectPartText(data?.output);
}

function sendJSON(res, statusCode, payload) {
  res.writeHead(statusCode, { 'content-type': 'application/json; charset=utf-8' });
  res.end(JSON.stringify(payload));
}

function sendOpenAIResponse(res, reqBody, text) {
  const id = `chatcmpl-joybuild-${Date.now()}`;
  if (reqBody.stream) {
    res.writeHead(200, {
      'content-type': 'text/event-stream; charset=utf-8',
      'cache-control': 'no-cache',
      connection: 'keep-alive',
    });
    res.write(`data: ${JSON.stringify({
      id,
      object: 'chat.completion.chunk',
      created: Math.floor(Date.now() / 1000),
      model: reqBody.model || defaultModel,
      choices: [{ index: 0, delta: { role: 'assistant', content: text }, finish_reason: null }],
    })}\n\n`);
    res.write(`data: ${JSON.stringify({
      id,
      object: 'chat.completion.chunk',
      created: Math.floor(Date.now() / 1000),
      model: reqBody.model || defaultModel,
      choices: [{ index: 0, delta: {}, finish_reason: 'stop' }],
    })}\n\n`);
    res.end('data: [DONE]\n\n');
    return;
  }
  sendJSON(res, 200, {
    id,
    object: 'chat.completion',
    created: Math.floor(Date.now() / 1000),
    model: reqBody.model || defaultModel,
    choices: [{ index: 0, message: { role: 'assistant', content: text }, finish_reason: 'stop' }],
  });
}

const server = http.createServer(async (req, res) => {
  try {
    if (req.method === 'GET' && req.url === '/health') {
      sendJSON(res, 200, { ok: true });
      return;
    }
    const requestPath = req.url?.split('?')[0].replace(/\/$/, '');
    if (req.method !== 'POST' || !['/v1/chat/completions', '/chat/completions'].includes(requestPath)) {
      console.error('Proxy unmatched request', req.method, req.url);
      sendJSON(res, 404, { error: { message: 'not found' } });
      return;
    }
    const openAIReq = await readJson(req);
    const joyBuildReq = buildJoyBuildRequest(openAIReq);
    const upstream = await fetch(`${baseUrl}/v1/responses`, {
      method: 'POST',
      headers: {
        authorization: `Bearer ${apiKey}`,
        'content-type': 'application/json',
        'trace-id': 'coze-loop-joybuild-proxy',
      },
      body: JSON.stringify(joyBuildReq),
    });
    const textBody = await upstream.text();
    let data;
    try {
      data = textBody ? JSON.parse(textBody) : {};
    } catch {
      data = { text: textBody };
    }
    if (!upstream.ok || data.error) {
      console.error('JoyBuild upstream error', upstream.status, JSON.stringify(data.error || data).slice(0, 500));
      sendJSON(res, upstream.status || 502, { error: data.error || { message: textBody || 'JoyBuild upstream error' } });
      return;
    }
    const text = extractJoyBuildText(data);
    if (!text) {
      console.error('JoyBuild empty text response', JSON.stringify(data).slice(0, 500));
      sendJSON(res, 502, { error: { message: 'JoyBuild returned no text content' } });
      return;
    }
    sendOpenAIResponse(res, openAIReq, text);
  } catch (err) {
    sendJSON(res, 500, { error: { message: err instanceof Error ? err.message : String(err) } });
  }
});

server.listen(port, host, () => {
  console.log(`JoyBuild OpenAI-compatible proxy listening on http://${host}:${port}/v1/chat/completions`);
});
