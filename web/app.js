const form = document.getElementById('workflow-form');
const submitBtn = document.getElementById('submit-btn');
const resultPanel = document.getElementById('result');
const resultHint = document.getElementById('result-hint');
const statusList = document.getElementById('status-list');
const statusHint = document.getElementById('status-hint');
const summary = document.getElementById('summary');
const summaryLine = document.getElementById('summary-line');
const summaryResults = document.getElementById('summary-results');

let socket = null;

function setResult(message, type = 'info') {
  resultPanel.textContent = message;
  resultPanel.dataset.type = type;
}

function resetStatus() {
  statusList.innerHTML = '';
  statusHint.textContent = 'Waiting for step updates...';
  delete statusHint.dataset.type;
  summary.hidden = true;
  summaryLine.textContent = '';
  summaryResults.innerHTML = '';
}

function getWebSocketUrl(workflowId) {
  const base = new URL('http://localhost:8080');
  const protocol = base.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${protocol}//${base.host}/ws/workflows/${workflowId}`;
}

function renderSteps(steps) {
  statusList.innerHTML = '';
  steps
    .sort((a, b) => a.step_number - b.step_number)
    .forEach((step) => {
      const item = document.createElement('li');
      item.textContent = `Step ${step.step_number}: ${step.tool} — ${step.status}`;
      statusList.appendChild(item);
    });
}

function formatLabel(value) {
  return value
    .replace(/_/g, ' ')
    .replace(/\b\w/g, (character) => character.toUpperCase());
}

function parseJsonString(value) {
  const trimmed = value.trim();
  if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) {
    return value;
  }

  try {
    return JSON.parse(trimmed);
  } catch {
    return value;
  }
}

function renderValue(value) {
  if (typeof value === 'string') {
    const parsed = parseJsonString(value);
    if (parsed !== value) {
      return renderValue(parsed);
    }

    if (/^https?:\/\//i.test(value)) {
      const link = document.createElement('a');
      link.href = value;
      link.textContent = value;
      link.target = '_blank';
      link.rel = 'noopener noreferrer';
      return link;
    }

    const text = document.createElement('span');
    text.textContent = value;
    return text;
  }

  if (Array.isArray(value)) {
    const list = document.createElement('ol');
    list.className = 'structured-list';
    value.forEach((item) => {
      const listItem = document.createElement('li');
      listItem.appendChild(renderValue(item));
      list.appendChild(listItem);
    });
    return list;
  }

  if (value && typeof value === 'object') {
    const fields = document.createElement('dl');
    fields.className = 'structured-fields';
    Object.entries(value).forEach(([key, fieldValue]) => {
      const term = document.createElement('dt');
      term.textContent = formatLabel(key);
      const detail = document.createElement('dd');
      detail.appendChild(renderValue(fieldValue));
      fields.append(term, detail);
    });
    return fields;
  }

  const text = document.createElement('span');
  text.textContent = value == null ? 'None' : String(value);
  return text;
}

function renderSearchAnswer(output) {
  if (!output || typeof output.answer !== 'string' || !output.answer.trim()) {
    return null;
  }

  const container = document.createElement('div');
  container.className = 'search-answer';

  const answer = document.createElement('p');
  answer.textContent = output.answer.trim();
  container.appendChild(answer);

  const source = Array.isArray(output.results) ? output.results[0] : null;
  if (source?.url) {
    const citation = document.createElement('a');
    citation.href = source.url;
    citation.textContent = source.title ? `Source: ${source.title}` : 'View source';
    citation.target = '_blank';
    citation.rel = 'noopener noreferrer';
    container.appendChild(citation);
  }

  return container;
}

function hasSearchAnswer(output) {
  return Boolean(output && typeof output.answer === 'string' && output.answer.trim());
}

function renderSummary(summaryData) {
  if (!summaryData) {
    return;
  }

  summary.hidden = false;
  const hasSingleAnswer = summaryData.results.length === 1
    && hasSearchAnswer(summaryData.results[0].output);
  summaryLine.textContent = hasSingleAnswer
    ? ''
    : `Completed ${summaryData.total} steps (${summaryData.succeeded} succeeded, ${summaryData.failed} failed).`;
  summaryResults.innerHTML = '';

  summaryData.results.forEach((result) => {
    const card = document.createElement('div');
    card.className = 'result-card';

    const title = document.createElement('strong');
    title.textContent = hasSingleAnswer
      ? 'Answer'
      : `Step ${result.step_number}: ${result.tool} — ${result.status}`;
    card.appendChild(title);

    if (result.error) {
      const error = document.createElement('div');
      error.textContent = `Error: ${result.error}`;
      card.appendChild(error);
    }

    if (result.output) {
      const output = document.createElement('div');
      output.className = 'structured-output';
      output.appendChild(renderSearchAnswer(result.output) || renderValue(result.output));
      card.appendChild(output);
    }

    summaryResults.appendChild(card);
  });
}

function connectWebSocket(workflowId) {
  if (socket) {
    socket.close();
  }

  const wsUrl = getWebSocketUrl(workflowId);
  const connection = new WebSocket(wsUrl);
  let terminalUpdateReceived = false;
  socket = connection;

  connection.addEventListener('open', () => {
    if (connection !== socket) {
      return;
    }
    statusHint.textContent = 'Connected. Waiting for step updates...';
  });

  connection.addEventListener('message', (event) => {
    if (connection !== socket) {
      return;
    }

    const payload = JSON.parse(event.data);

    if (payload.error) {
      terminalUpdateReceived = true;
      statusHint.textContent = `Workflow update failed: ${payload.error}`;
      statusHint.dataset.type = 'error';
      return;
    }

    renderSteps(payload.steps || []);

    if (payload.workflow_status === 'failed') {
      terminalUpdateReceived = true;
      statusHint.textContent = payload.workflow_error
        ? `Workflow failed: ${payload.workflow_error}`
        : 'Workflow failed.';
      statusHint.dataset.type = 'error';
      renderSummary(payload.summary);
      connection.close();
      return;
    }

    if (payload.completed) {
      terminalUpdateReceived = true;
      statusHint.textContent = 'Workflow completed.';
      statusHint.dataset.type = 'success';
      renderSummary(payload.summary);
      connection.close();
    }
  });

  connection.addEventListener('close', () => {
    if (connection !== socket || terminalUpdateReceived) {
      return;
    }
    statusHint.textContent = 'Lost connection while waiting for workflow updates.';
    statusHint.dataset.type = 'error';
  });

  connection.addEventListener('error', () => {
    if (connection !== socket || terminalUpdateReceived) {
      return;
    }
    statusHint.textContent = 'Could not connect to workflow updates.';
    statusHint.dataset.type = 'error';
  });
}

form.addEventListener('submit', async (event) => {
  event.preventDefault();
  const requestText = form.request.value.trim();

  if (!requestText) {
    setResult('Please enter a workflow request.', 'error');
    return;
  }

  submitBtn.disabled = true;
  setResult('Creating workflow...', 'info');
  resetStatus();

  try {
    const response = await fetch(`${'http://localhost:8080'}/workflow`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ request: requestText }),
    });

    if (!response.ok) {
      const text = await response.text();
      throw new Error(text || 'Failed to create workflow');
    }

    const data = await response.json();
    const workflowId = data.workflow_id || data.workflowId;

    if (!workflowId) {
      throw new Error('Missing workflow_id in response');
    }

    resultHint.textContent = 'Workflow created successfully.';
    setResult(`Workflow ID: ${workflowId}`, 'success');
    connectWebSocket(workflowId);
  } catch (error) {
    setResult(error.message, 'error');
  } finally {
    submitBtn.disabled = false;
  }
});
