(function () {
  const root = document.getElementById("terminal");
  if (!root) return;

  const term = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily:
      'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
    theme: {
      background: "#0b0e14",
      foreground: "#e6e1cf",
    },
  });

  const fitAddon = new FitAddon.FitAddon();
  term.loadAddon(fitAddon);
  term.open(root);

  const wsProto = window.location.protocol === "https:" ? "wss://" : "ws://";
  const ws = new WebSocket(wsProto + window.location.host + "/ws");
  ws.binaryType = "arraybuffer";

  function sendResize() {
    if (ws.readyState !== WebSocket.OPEN) return;
    ws.send(
      JSON.stringify({
        type: "resize",
        cols: term.cols,
        rows: term.rows,
      })
    );
  }

  function fit() {
    fitAddon.fit();
    sendResize();
  }

  ws.onopen = function () {
    fit();
    term.focus();
  };

  ws.onmessage = function (ev) {
    if (typeof ev.data === "string") {
      term.write(ev.data);
      return;
    }
    const bytes = new Uint8Array(ev.data);
    term.write(bytes);
  };

  ws.onclose = function () {
    term.write("\r\n\r\n[disconnected]\r\n");
  };

  ws.onerror = function () {
    term.write("\r\n\r\n[websocket error]\r\n");
  };

  term.onData(function (data) {
    if (ws.readyState !== WebSocket.OPEN) return;
    ws.send(data);
  });

  window.addEventListener("resize", function () {
    fit();
  });
})();
