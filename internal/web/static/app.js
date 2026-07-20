(() => {
  const loginView = document.getElementById("login-view");
  const appView = document.getElementById("app-view");
  const loginForm = document.getElementById("login-form");
  const loginPassword = document.getElementById("login-password");
  const loginError = document.getElementById("login-error");
  const logoutBtn = document.getElementById("logout-btn");
  const dropzone = document.getElementById("dropzone");
  const fileInput = document.getElementById("file-input");
  const uploadsList = document.getElementById("uploads");
  const fileListBody = document.getElementById("file-list");
  const emptyState = document.getElementById("empty-state");

  const ICON_DOWNLOAD = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3v12"/><path d="m7 10 5 5 5-5"/><path d="M5 21h14"/></svg>';
  const ICON_RENAME = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 3a2.85 2.85 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5Z"/></svg>';
  const ICON_TRASH = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 7h16"/><path d="M6 7v13a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1V7"/><path d="M9 7V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v3"/></svg>';

  function showLogin() {
    loginView.hidden = false;
    appView.hidden = true;
  }

  function formatSize(bytes) {
    if (bytes < 1024) return bytes + " B";
    const units = ["KB", "MB", "GB", "TB"];
    let n = bytes / 1024;
    let i = 0;
    while (n >= 1024 && i < units.length - 1) {
      n /= 1024;
      i++;
    }
    return n.toFixed(n < 10 ? 1 : 0) + " " + units[i];
  }

  function formatDate(iso) {
    const d = new Date(iso);
    return d.toLocaleString(undefined, {
      year: "numeric", month: "short", day: "numeric",
      hour: "2-digit", minute: "2-digit",
    });
  }

  async function loadFiles() {
    const res = await fetch("/api/files");
    if (res.status === 401) {
      showLogin();
      return;
    }
    loginView.hidden = true;
    appView.hidden = false;
    const records = await res.json();
    renderFiles(records || []);
  }

  function renderFiles(records) {
    fileListBody.innerHTML = "";
    emptyState.hidden = records.length > 0;
    for (const rec of records) {
      fileListBody.appendChild(renderFileRow(rec));
    }
  }

  function renderFileRow(rec) {
    const li = document.createElement("li");
    li.className = "file-row";

    const info = document.createElement("div");
    info.className = "file-info";

    const name = document.createElement("div");
    name.className = "file-name";
    name.textContent = rec.Filename;
    info.appendChild(name);

    const meta = document.createElement("div");
    meta.className = "file-meta";
    meta.textContent = formatSize(rec.Size) + " · " + formatDate(rec.UploadedAt);
    info.appendChild(meta);

    li.appendChild(info);

    const actions = document.createElement("div");
    actions.className = "file-actions";

    const downloadLink = document.createElement("a");
    downloadLink.href = "/api/files/" + rec.ID;
    downloadLink.className = "icon-btn";
    downloadLink.setAttribute("aria-label", "Download " + rec.Filename);
    downloadLink.innerHTML = ICON_DOWNLOAD;
    actions.appendChild(downloadLink);

    const renameBtn = document.createElement("button");
    renameBtn.type = "button";
    renameBtn.className = "icon-btn";
    renameBtn.setAttribute("aria-label", "Rename " + rec.Filename);
    renameBtn.innerHTML = ICON_RENAME;
    renameBtn.addEventListener("click", () => renameFile(rec, name));
    actions.appendChild(renameBtn);

    const deleteBtn = document.createElement("button");
    deleteBtn.type = "button";
    deleteBtn.className = "icon-btn danger";
    deleteBtn.setAttribute("aria-label", "Delete " + rec.Filename);
    deleteBtn.innerHTML = ICON_TRASH;
    deleteBtn.addEventListener("click", () => deleteFile(rec, li));
    actions.appendChild(deleteBtn);

    li.appendChild(actions);
    return li;
  }

  async function renameFile(rec, nameEl) {
    const next = prompt("Rename file", rec.Filename);
    if (next === null) return;
    const filename = next.trim();
    if (!filename || filename === rec.Filename) return;

    const res = await fetch("/api/files/" + rec.ID, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ filename }),
    });
    if (res.status === 401) {
      showLogin();
      return;
    }
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      alert(body.error || "Failed to rename file");
      return;
    }
    const updated = await res.json();
    rec.Filename = updated.Filename;
    nameEl.textContent = updated.Filename;
  }

  async function deleteFile(rec, rowEl) {
    if (!confirm('Delete "' + rec.Filename + '"? This removes it from Telegram too.')) {
      return;
    }
    const res = await fetch("/api/files/" + rec.ID, { method: "DELETE" });
    if (res.status === 401) {
      showLogin();
      return;
    }
    if (!res.ok && res.status !== 204) {
      const body = await res.json().catch(() => ({}));
      alert(body.error || "Failed to delete file");
      return;
    }
    rowEl.remove();
    emptyState.hidden = fileListBody.children.length > 0;
  }

  function uploadFile(file) {
    const item = document.createElement("li");
    item.innerHTML =
      '<div class="upload-name"><span>' + escapeHTML(file.name) +
      '</span><span class="status">0%</span></div>' +
      '<div class="progress-track"><div class="progress-fill"></div></div>';
    uploadsList.appendChild(item);

    const status = item.querySelector(".status");
    const fill = item.querySelector(".progress-fill");

    return new Promise((resolve) => {
      const xhr = new XMLHttpRequest();
      xhr.open("POST", "/api/files");

      xhr.upload.onprogress = (e) => {
        if (!e.lengthComputable) return;
        const pct = Math.round((e.loaded / e.total) * 100);
        fill.style.width = pct + "%";
        status.textContent = pct + "%";
      };

      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          status.textContent = "Done";
          setTimeout(() => item.remove(), 1500);
        } else if (xhr.status === 401) {
          status.textContent = "Session expired";
          status.classList.add("error-text");
          showLogin();
        } else {
          let msg = "Failed";
          try { msg = JSON.parse(xhr.responseText).error || msg; } catch {}
          status.textContent = msg;
          status.classList.add("error-text");
        }
        resolve();
      };

      xhr.onerror = () => {
        status.textContent = "Network error";
        status.classList.add("error-text");
        resolve();
      };

      const form = new FormData();
      form.append("file", file);
      xhr.send(form);
    });
  }

  function escapeHTML(s) {
    const div = document.createElement("div");
    div.textContent = s;
    return div.innerHTML;
  }

  async function uploadFiles(fileList) {
    const files = Array.from(fileList);
    for (const file of files) {
      await uploadFile(file);
    }
    loadFiles();
  }

  dropzone.addEventListener("click", () => fileInput.click());
  fileInput.addEventListener("change", () => {
    if (fileInput.files.length) uploadFiles(fileInput.files);
    fileInput.value = "";
  });

  ["dragenter", "dragover"].forEach((evt) => {
    dropzone.addEventListener(evt, (e) => {
      e.preventDefault();
      dropzone.classList.add("drag-over");
    });
  });
  ["dragleave", "drop"].forEach((evt) => {
    dropzone.addEventListener(evt, (e) => {
      e.preventDefault();
      dropzone.classList.remove("drag-over");
    });
  });
  dropzone.addEventListener("drop", (e) => {
    if (e.dataTransfer.files.length) uploadFiles(e.dataTransfer.files);
  });

  loginForm.addEventListener("submit", async (e) => {
    e.preventDefault();
    loginError.hidden = true;
    const res = await fetch("/api/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password: loginPassword.value }),
    });
    if (res.ok) {
      loginPassword.value = "";
      loadFiles();
      return;
    }
    const body = await res.json().catch(() => ({}));
    loginError.textContent = body.error || "Login failed";
    loginError.hidden = false;
  });

  logoutBtn.addEventListener("click", async () => {
    await fetch("/api/logout", { method: "POST" });
    showLogin();
  });

  loadFiles();

  if ("serviceWorker" in navigator) {
    navigator.serviceWorker.register("/sw.js");
  }
})();
