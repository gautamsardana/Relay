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

function renderSummary(summaryData) {
  if (!summaryData) {
    return;
  }

  summary.hidden = false;
  summaryLine.textContent = `Completed ${summaryData.total} steps (${summaryData.succeeded} succeeded, ${summaryData.failed} failed).`;
  summaryResults.innerHTML = '';

  summaryData.results.forEach((result) => {
    const card = document.createElement('div');
    card.className = 'result-card';

    const title = document.createElement('strong');
    title.textContent = `Step ${result.step_number}: ${result.tool} — ${result.status}`;
    card.appendChild(title);

    if (result.error) {
      const error = document.createElement('div');
      error.textContent = `Error: ${result.error}`;
      card.appendChild(error);
    }

    if (result.output) {
      const output = document.createElement('pre');
      output.textContent = JSON.stringify(result.output, null, 2);
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
  socket = new WebSocket(wsUrl);

  socket.addEventListener('open', () => {
    statusHint.textContent = 'Connected. Waiting for step updates...';
  });

  socket.addEventListener('message', (event) => {
    const payload = JSON.parse(event.data);

    if (payload.error) {
      statusHint.textContent = payload.error;
      return;
    }

    renderSteps(payload.steps || []);

    if (payload.completed) {
      statusHint.textContent = 'All steps completed.';
      renderSummary(payload.summary);
      socket.close();
    }
  });

  socket.addEventListener('close', () => {
    if (!summary.hidden) {
      return;
    }
    statusHint.textContent = 'Websocket closed.';
  });

  socket.addEventListener('error', () => {
    statusHint.textContent = 'Websocket error.';
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
