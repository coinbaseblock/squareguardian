package api

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// faceProxy forwards requests to the face-service.
func (h *Handler) faceProxy(w http.ResponseWriter, r *http.Request) {
	if h.faceServiceURL == "" {
		http.Error(w, `{"error":"face-service not configured"}`, http.StatusServiceUnavailable)
		return
	}

	// Forward the request path as-is to face-service
	targetURL := h.faceServiceURL + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to create proxy request"}`, http.StatusInternalServerError)
		return
	}
	proxyReq.Header.Set("Content-Type", r.Header.Get("Content-Type"))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"face-service unavailable: %s"}`, err.Error()), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// facesPage renders the Face Gallery UI.
func (h *Handler) facesPage(w http.ResponseWriter, r *http.Request) {
	faceServiceStatus := "ไม่ได้เชื่อมต่อ"
	statusClass := "status-offline"
	if h.faceServiceURL != "" {
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(h.faceServiceURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				faceServiceStatus = "พร้อมใช้งาน"
				statusClass = "status-online"
			}
		}
	}

	// Get nav bar consistent with other pages
	navLinks := []struct{ href, label, active string }{
		{"/", "Dashboard", ""},
		{"/events", "Events", ""},
		{"/faces", "Face Gallery", " active"},
	}
	navHTML := ""
	for _, n := range navLinks {
		navHTML += fmt.Sprintf(`<a href="%s" class="nav-link%s">%s</a>`, n.href, n.active, n.label)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, facesPageTpl, navHTML, statusClass, faceServiceStatus)
}

var facesPageTpl = `<!DOCTYPE html>
<html lang="th">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Face Gallery — SquareGuardian</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0f1117; color: #e0e0e0; }
  .nav { background: #1a1d27; padding: 0.8em 1.5em; display: flex; align-items: center; gap: 1em; border-bottom: 1px solid #2a2d37; }
  .nav-brand { font-weight: bold; font-size: 1.1em; color: #60a5fa; margin-right: 1em; }
  .nav-link { color: #9ca3af; text-decoration: none; padding: 0.4em 0.8em; border-radius: 6px; }
  .nav-link:hover { background: #2a2d37; color: #e0e0e0; }
  .nav-link.active { background: #2563eb; color: white; }
  .container { max-width: 1200px; margin: 0 auto; padding: 1.5em; }
  h1 { margin-bottom: 0.5em; font-size: 1.5em; }
  .status-bar { display: flex; align-items: center; gap: 0.5em; margin-bottom: 1.5em; padding: 0.6em 1em; background: #1a1d27; border-radius: 8px; }
  .status-dot { width: 10px; height: 10px; border-radius: 50%%; }
  .status-online .status-dot { background: #22c55e; }
  .status-offline .status-dot { background: #ef4444; }
  .section { background: #1a1d27; border-radius: 10px; padding: 1.5em; margin-bottom: 1.5em; }
  .section h2 { font-size: 1.1em; margin-bottom: 1em; color: #93c5fd; }
  .gallery { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 1em; }
  .person-card { background: #252830; border-radius: 10px; padding: 1em; text-align: center; transition: transform 0.2s; }
  .person-card:hover { transform: translateY(-2px); }
  .person-name { font-weight: bold; font-size: 1.1em; margin-bottom: 0.3em; }
  .person-info { color: #9ca3af; font-size: 0.85em; }
  .person-avatar { width: 80px; height: 80px; border-radius: 50%%; background: #374151; display: flex; align-items: center; justify-content: center; margin: 0 auto 0.8em; font-size: 2em; }
  .btn { padding: 0.5em 1.2em; border: none; border-radius: 6px; cursor: pointer; font-size: 0.9em; transition: background 0.2s; }
  .btn-primary { background: #2563eb; color: white; }
  .btn-primary:hover { background: #1d4ed8; }
  .btn-danger { background: #dc2626; color: white; }
  .btn-danger:hover { background: #b91c1c; }
  .btn-sm { padding: 0.3em 0.8em; font-size: 0.8em; }
  .register-form { display: grid; gap: 1em; max-width: 500px; }
  .register-form label { font-weight: 500; margin-bottom: 0.3em; display: block; }
  .register-form input[type="text"] { width: 100%%; padding: 0.5em; background: #252830; border: 1px solid #374151; border-radius: 6px; color: #e0e0e0; font-size: 1em; }
  .register-form input[type="file"] { color: #9ca3af; }
  .empty-state { text-align: center; color: #6b7280; padding: 3em 1em; }
  .empty-state p { margin-bottom: 1em; }
  .toast { position: fixed; bottom: 1.5em; right: 1.5em; padding: 0.8em 1.5em; border-radius: 8px; color: white; display: none; z-index: 100; }
  .toast-success { background: #16a34a; }
  .toast-error { background: #dc2626; }
  .actions { display: flex; gap: 0.5em; justify-content: center; margin-top: 0.5em; }
</style>
</head>
<body>
<nav class="nav">
  <span class="nav-brand">SquareGuardian</span>
  %s
</nav>

<div class="container">
  <h1>Face Gallery</h1>
  <div class="status-bar %s">
    <span class="status-dot"></span>
    <span>Face Service: %s</span>
  </div>

  <div class="section">
    <h2>ลงทะเบียนบุคคลใหม่</h2>
    <div class="register-form">
      <div>
        <label for="personName">ชื่อบุคคล</label>
        <input type="text" id="personName" placeholder="เช่น สมชาย, พนักงานส่งของ">
      </div>
      <div>
        <label for="faceImages">ภาพใบหน้า (1-5 ภาพ)</label>
        <input type="file" id="faceImages" accept="image/*" multiple>
      </div>
      <div>
        <button class="btn btn-primary" onclick="registerPerson()">ลงทะเบียน</button>
      </div>
    </div>
  </div>

  <div class="section">
    <h2>บุคคลที่ลงทะเบียนแล้ว</h2>
    <div id="galleryContainer">
      <div class="empty-state">
        <p>กำลังโหลด...</p>
      </div>
    </div>
  </div>
</div>

<div id="toast" class="toast"></div>

<script>
async function loadGallery() {
  const container = document.getElementById('galleryContainer');
  try {
    const resp = await fetch('/api/face/gallery');
    if (!resp.ok) throw new Error('Face service unavailable');
    const data = await resp.json();
    if (!data.persons || data.persons.length === 0) {
      container.innerHTML = '<div class="empty-state"><p>ยังไม่มีบุคคลที่ลงทะเบียน</p><p>เพิ่มบุคคลใหม่โดยกรอกชื่อและอัปโหลดภาพด้านบน</p></div>';
      return;
    }
    const initials = (name) => name.charAt(0).toUpperCase();
    container.innerHTML = '<div class="gallery">' + data.persons.map(p =>
      '<div class="person-card">' +
        '<div class="person-avatar">' + initials(p.name) + '</div>' +
        '<div class="person-name">' + escapeHtml(p.name) + '</div>' +
        '<div class="person-info">' + p.face_count + ' embeddings</div>' +
        '<div class="person-info">แหล่ง: ' + escapeHtml(p.source) + '</div>' +
        '<div class="actions">' +
          '<button class="btn btn-danger btn-sm" onclick="deletePerson(\'' + p.id + '\',\'' + escapeHtml(p.name) + '\')">ลบ</button>' +
        '</div>' +
      '</div>'
    ).join('') + '</div>';
  } catch (e) {
    container.innerHTML = '<div class="empty-state"><p>ไม่สามารถเชื่อมต่อ Face Service ได้</p><p>' + escapeHtml(e.message) + '</p></div>';
  }
}

async function registerPerson() {
  const name = document.getElementById('personName').value.trim();
  const files = document.getElementById('faceImages').files;
  if (!name) { showToast('กรุณาใส่ชื่อบุคคล', 'error'); return; }
  if (files.length === 0) { showToast('กรุณาเลือกภาพอย่างน้อย 1 ภาพ', 'error'); return; }

  const images = [];
  for (const file of files) {
    const b64 = await fileToBase64(file);
    images.push(b64);
  }

  try {
    const resp = await fetch('/api/face/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, images })
    });
    const data = await resp.json();
    if (!resp.ok) throw new Error(data.detail || 'Registration failed');
    showToast('ลงทะเบียน "' + name + '" สำเร็จ (' + data.embeddings_count + ' embeddings)', 'success');
    document.getElementById('personName').value = '';
    document.getElementById('faceImages').value = '';
    loadGallery();
  } catch (e) {
    showToast('เกิดข้อผิดพลาด: ' + e.message, 'error');
  }
}

async function deletePerson(id, name) {
  if (!confirm('ต้องการลบ "' + name + '" หรือไม่?')) return;
  try {
    const resp = await fetch('/api/face/gallery/' + id, { method: 'DELETE' });
    if (!resp.ok) throw new Error('Delete failed');
    showToast('ลบ "' + name + '" แล้ว', 'success');
    loadGallery();
  } catch (e) {
    showToast('เกิดข้อผิดพลาด: ' + e.message, 'error');
  }
}

function fileToBase64(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result.split(',')[1]);
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
}

function escapeHtml(s) {
  const d = document.createElement('div');
  d.textContent = s;
  return d.innerHTML;
}

function showToast(msg, type) {
  const t = document.getElementById('toast');
  t.textContent = msg;
  t.className = 'toast toast-' + type;
  t.style.display = 'block';
  setTimeout(() => { t.style.display = 'none'; }, 4000);
}

loadGallery();
</script>
</body>
</html>
`
