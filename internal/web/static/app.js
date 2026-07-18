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
      const tr = document.createElement("tr");

      const nameTd = document.createElement("td");
      nameTd.textContent = rec.Filename;
      tr.appendChild(nameTd);

      const sizeTd = document.createElement("td");
      sizeTd.textContent = formatSize(rec.Size);
      tr.appendChild(sizeTd);

      const dateTd = document.createElement("td");
      dateTd.textContent = formatDate(rec.UploadedAt);
      tr.appendChild(dateTd);

      const actionTd = document.createElement("td");
      const link = document.createElement("a");
      link.href = "/api/files/" + rec.ID;
      link.textContent = "Download";
      link.className = "download-link";
      actionTd.appendChild(link);
      tr.appendChild(actionTd);

      fileListBody.appendChild(tr);
    }
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
})();
