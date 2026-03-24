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

	navHTML := `<a href="/" class="nav-link" data-i18n="dashboard">Dashboard</a>` +
		`<a href="/events" class="nav-link" data-i18n="events_by_type">Events</a>` +
		`<a href="/faces" class="nav-link active" data-i18n="face_gallery">Face Gallery</a>` +
		`<button class="lang-btn" id="langToggle" onclick="toggleLang()" style="margin-left:auto;background:#252836;border:1px solid #555;color:#4fc3f7;padding:4px 12px;border-radius:6px;cursor:pointer;font-size:.8em;font-weight:bold">EN</button>`

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
  .btn-success { background: #16a34a; color: white; }
  .btn-success:hover { background: #15803d; }
  .btn-secondary { background: #4b5563; color: white; }
  .btn-secondary:hover { background: #374151; }
  .btn-sm { padding: 0.3em 0.8em; font-size: 0.8em; }
  .btn:disabled { opacity: 0.5; cursor: not-allowed; }
  .register-form { display: grid; gap: 1em; max-width: 500px; }
  .register-form label { font-weight: 500; margin-bottom: 0.3em; display: block; }
  .register-form input[type="text"], .register-form select { width: 100%%; padding: 0.5em; background: #252830; border: 1px solid #374151; border-radius: 6px; color: #e0e0e0; font-size: 1em; }
  .register-form input[type="file"] { color: #9ca3af; }
  .empty-state { text-align: center; color: #6b7280; padding: 3em 1em; }
  .empty-state p { margin-bottom: 1em; }
  .toast { position: fixed; bottom: 1.5em; right: 1.5em; padding: 0.8em 1.5em; border-radius: 8px; color: white; display: none; z-index: 100; }
  .toast-success { background: #16a34a; }
  .toast-error { background: #dc2626; }
  .actions { display: flex; gap: 0.5em; justify-content: center; margin-top: 0.5em; }

  /* Tabs */
  .tabs { display: flex; gap: 0; margin-bottom: 1.2em; border-bottom: 2px solid #2a2d37; }
  .tab { padding: 0.6em 1.2em; cursor: pointer; color: #9ca3af; border-bottom: 2px solid transparent; margin-bottom: -2px; transition: all 0.2s; background: none; border-top: none; border-left: none; border-right: none; font-size: 0.95em; }
  .tab:hover { color: #e0e0e0; }
  .tab.active { color: #60a5fa; border-bottom-color: #60a5fa; }
  .tab-content { display: none; }
  .tab-content.active { display: block; }

  /* Camera capture */
  .camera-area { display: flex; gap: 1.5em; flex-wrap: wrap; }
  .camera-preview { position: relative; background: #000; border-radius: 8px; overflow: hidden; width: 480px; max-width: 100%%; aspect-ratio: 16/9; }
  .camera-preview img { width: 100%%; height: 100%%; object-fit: contain; }
  .camera-preview .no-feed { display: flex; align-items: center; justify-content: center; width: 100%%; height: 100%%; color: #6b7280; }
  .pose-panel { flex: 1; min-width: 250px; }
  .pose-guide { margin-bottom: 1em; }
  .pose-step { display: flex; align-items: center; gap: 0.8em; padding: 0.6em 0.8em; border-radius: 8px; margin-bottom: 0.4em; transition: background 0.2s; }
  .pose-step.current { background: #1e3a5f; }
  .pose-step.done { opacity: 0.6; }
  .pose-icon { width: 36px; height: 36px; border-radius: 50%%; display: flex; align-items: center; justify-content: center; font-size: 1.2em; flex-shrink: 0; }
  .pose-step.pending .pose-icon { background: #374151; }
  .pose-step.current .pose-icon { background: #2563eb; }
  .pose-step.done .pose-icon { background: #16a34a; }
  .pose-label { font-size: 0.9em; }
  .pose-hint { font-size: 0.75em; color: #9ca3af; }
  .captured-shots { display: flex; gap: 0.5em; flex-wrap: wrap; margin-top: 1em; }
  .captured-thumb { width: 72px; height: 72px; border-radius: 8px; overflow: hidden; border: 2px solid #374151; position: relative; }
  .captured-thumb img { width: 100%%; height: 100%%; object-fit: cover; }
  .captured-thumb.active { border-color: #2563eb; }
  .capture-actions { display: flex; gap: 0.5em; margin-top: 1em; flex-wrap: wrap; }
  .face-note { background: #1e293b; border-radius: 8px; padding: 0.8em 1em; margin-bottom: 1em; font-size: 0.85em; color: #94a3b8; border-left: 3px solid #3b82f6; }

  /* Detection history */
  .detection-person { margin-bottom: 1.5em; }
  .detection-person-header { display: flex; align-items: center; gap: 0.8em; margin-bottom: 0.8em; padding-bottom: 0.5em; border-bottom: 1px solid #2a2d37; }
  .detection-person-name { font-weight: bold; font-size: 1.05em; }
  .detection-person-count { color: #9ca3af; font-size: 0.85em; }
  .detection-person-last { color: #60a5fa; font-size: 0.8em; margin-left: auto; }
  .detection-grid { display: flex; gap: 0.6em; flex-wrap: wrap; }
  .detection-card { background: #252830; border-radius: 8px; overflow: hidden; width: 140px; transition: transform 0.2s; cursor: pointer; }
  .detection-card:hover { transform: translateY(-2px); }
  .detection-card img { width: 100%%; height: 90px; object-fit: cover; }
  .detection-card-info { padding: 0.4em 0.6em; font-size: 0.75em; }
  .detection-card-time { color: #9ca3af; }
  .detection-card-camera { color: #60a5fa; }
  .detection-card-score { color: #22c55e; font-size: 0.7em; }
  .detection-badge { display: inline-block; background: #16a34a; color: white; padding: 0.15em 0.5em; border-radius: 4px; font-size: 0.75em; margin-left: 0.5em; }
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

    <div class="face-note">
      ระบบจดจำใบหน้าใช้เฉพาะ <strong>ใบหน้า</strong> ในการแยกแยะบุคคล ไม่จำเป็นต้องถ่ายเต็มตัว — ถ่ายให้เห็นใบหน้าชัดเจนเพียงพอ
    </div>

    <div class="tabs">
      <button class="tab active" onclick="switchTab('upload')">อัปโหลดรูปภาพ</button>
      <button class="tab" onclick="switchTab('camera')">ถ่ายจากกล้อง</button>
    </div>

    <!-- Tab 1: Upload -->
    <div id="tab-upload" class="tab-content active">
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

    <!-- Tab 2: Camera Capture -->
    <div id="tab-camera" class="tab-content">
      <div class="register-form" style="max-width:100%%">
        <div style="display:flex; gap:1em; flex-wrap:wrap; align-items:end;">
          <div style="flex:1; min-width:200px;">
            <label for="camPersonName">ชื่อบุคคล</label>
            <input type="text" id="camPersonName" placeholder="เช่น สมชาย, พนักงานส่งของ">
          </div>
          <div style="flex:1; min-width:200px;">
            <label for="cameraSelect">เลือกกล้อง</label>
            <select id="cameraSelect" onchange="startCameraPreview()">
              <option value="">-- กำลังโหลดรายชื่อกล้อง --</option>
            </select>
          </div>
        </div>

        <div class="camera-area">
          <div class="camera-preview" id="cameraPreview">
            <div class="no-feed" id="noFeed">เลือกกล้องเพื่อเริ่มดูภาพ</div>
            <img id="cameraImg" style="display:none" alt="Camera feed">
          </div>

          <div class="pose-panel">
            <h3 style="font-size:0.95em; margin-bottom:0.6em; color:#93c5fd;">ท่าถ่ายภาพ</h3>
            <div class="pose-guide" id="poseGuide">
              <!-- Populated by JS -->
            </div>

            <div class="capture-actions">
              <button class="btn btn-primary" id="captureBtn" onclick="captureShot()" disabled>ถ่ายภาพ</button>
              <button class="btn btn-secondary" id="skipBtn" onclick="skipPose()" disabled>ข้ามท่านี้</button>
              <button class="btn btn-success" id="camRegisterBtn" onclick="registerFromCamera()" disabled>ลงทะเบียน</button>
            </div>

            <div class="captured-shots" id="capturedShots"></div>
          </div>
        </div>
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

  <div class="section">
    <h2>ประวัติการตรวจพบ</h2>
    <div class="face-note">
      เมื่อกล้องตรวจพบบุคคลที่ลงทะเบียนแล้ว ระบบจะระบุตัวตนอัตโนมัติและแสดงผลที่นี่
    </div>
    <div id="detectionHistory">
      <div class="empty-state">
        <p>กำลังโหลด...</p>
      </div>
    </div>
  </div>
</div>

<div id="toast" class="toast"></div>

<script>
// --- Pose definitions ---
const POSES = [
  { id: 'front',  label: 'มองตรง',     hint: 'มองตรงมาที่กล้อง',        icon: '正' },
  { id: 'left',   label: 'หันซ้าย',     hint: 'หันหน้าไปทางซ้ายเล็กน้อย', icon: '←' },
  { id: 'right',  label: 'หันขวา',      hint: 'หันหน้าไปทางขวาเล็กน้อย',  icon: '→' },
  { id: 'up',     label: 'เงยหน้าขึ้น',  hint: 'เงยหน้าขึ้นเล็กน้อย',      icon: '↑' },
  { id: 'down',   label: 'ก้มหน้าลง',   hint: 'ก้มหน้าลงเล็กน้อย',       icon: '↓' },
];

let currentPoseIdx = 0;
let capturedImages = []; // base64 strings
let cameraInterval = null;

// --- Tab switching ---
function switchTab(tab) {
  document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
  document.querySelectorAll('.tab-content').forEach(t => t.classList.remove('active'));
  document.getElementById('tab-' + tab).classList.add('active');
  document.querySelector('.tab[onclick*="' + tab + '"]').classList.add('active');
  if (tab === 'camera') loadCameras();
}

// --- Upload registration ---
async function loadGallery() {
  const container = document.getElementById('galleryContainer');
  try {
    const [galleryResp, identResp] = await Promise.all([
      fetch('/api/face/gallery'),
      fetch('/api/events/identified').catch(() => null)
    ]);
    if (!galleryResp.ok) throw new Error('Face service unavailable');
    const data = await galleryResp.json();
    const identData = identResp && identResp.ok ? await identResp.json() : {};

    if (!data.persons || data.persons.length === 0) {
      container.innerHTML = '<div class="empty-state"><p>ยังไม่มีบุคคลที่ลงทะเบียน</p><p>เพิ่มบุคคลใหม่โดยกรอกชื่อและอัปโหลดภาพด้านบน หรือถ่ายจากกล้อง</p></div>';
      return;
    }
    const initials = (name) => name.charAt(0).toUpperCase();
    container.innerHTML = '<div class="gallery">' + data.persons.map(p => {
      const detections = identData[p.name] || [];
      const detCount = detections.length;
      let detInfo = '<div class="person-info" style="color:#6b7280">ยังไม่เคยตรวจพบ</div>';
      if (detCount > 0) {
        const lastTime = timeAgo(detections[0].start_time);
        detInfo = '<div class="person-info" style="color:#22c55e">ตรวจพบ ' + detCount + ' ครั้ง</div>' +
                  '<div class="person-info" style="color:#60a5fa; font-size:0.75em">ล่าสุด: ' + lastTime + '</div>';
      }
      return '<div class="person-card">' +
        '<div class="person-avatar">' + initials(p.name) + '</div>' +
        '<div class="person-name">' + escapeHtml(p.name) + '</div>' +
        '<div class="person-info">' + p.face_count + ' embeddings</div>' +
        '<div class="person-info">แหล่ง: ' + escapeHtml(p.source) + '</div>' +
        detInfo +
        '<div class="actions">' +
          '<button class="btn btn-danger btn-sm" onclick="deletePerson(\'' + p.id + '\',\'' + escapeHtml(p.name) + '\')">ลบ</button>' +
        '</div>' +
      '</div>';
    }).join('') + '</div>';
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

// --- Camera capture ---
async function loadCameras() {
  const sel = document.getElementById('cameraSelect');
  try {
    const resp = await fetch('/api/cameras');
    const data = await resp.json();
    if (!data.cameras || data.cameras.length === 0) {
      sel.innerHTML = '<option value="">ไม่พบกล้อง</option>';
      return;
    }
    sel.innerHTML = '<option value="">-- เลือกกล้อง --</option>' +
      data.cameras.map(c => '<option value="' + escapeHtml(c) + '">' + escapeHtml(c) + '</option>').join('');
  } catch (e) {
    sel.innerHTML = '<option value="">โหลดรายชื่อกล้องไม่ได้</option>';
  }
}

function startCameraPreview() {
  const cam = document.getElementById('cameraSelect').value;
  const img = document.getElementById('cameraImg');
  const noFeed = document.getElementById('noFeed');

  if (cameraInterval) { clearInterval(cameraInterval); cameraInterval = null; }

  if (!cam) {
    img.style.display = 'none';
    noFeed.style.display = 'flex';
    document.getElementById('captureBtn').disabled = true;
    document.getElementById('skipBtn').disabled = true;
    return;
  }

  noFeed.style.display = 'none';
  img.style.display = 'block';

  function refreshFrame() {
    img.src = '/api/camera-snapshot/' + encodeURIComponent(cam) + '?t=' + Date.now();
  }
  refreshFrame();
  cameraInterval = setInterval(refreshFrame, 1000);

  // Reset pose state
  currentPoseIdx = 0;
  capturedImages = [];
  renderPoseGuide();
  renderCapturedThumbs();
  document.getElementById('captureBtn').disabled = false;
  document.getElementById('skipBtn').disabled = false;
  document.getElementById('camRegisterBtn').disabled = true;
}

function renderPoseGuide() {
  const guide = document.getElementById('poseGuide');
  guide.innerHTML = POSES.map((p, i) => {
    let cls = 'pose-step';
    if (i < currentPoseIdx) cls += ' done';
    else if (i === currentPoseIdx) cls += ' current';
    else cls += ' pending';
    return '<div class="' + cls + '">' +
      '<div class="pose-icon">' + (i < currentPoseIdx ? '&#10003;' : p.icon) + '</div>' +
      '<div><div class="pose-label">' + p.label + '</div><div class="pose-hint">' + p.hint + '</div></div>' +
    '</div>';
  }).join('');
}

function renderCapturedThumbs() {
  const container = document.getElementById('capturedShots');
  if (capturedImages.length === 0) {
    container.innerHTML = '';
    return;
  }
  container.innerHTML = capturedImages.map((b64, i) =>
    '<div class="captured-thumb"><img src="data:image/jpeg;base64,' + b64 + '"></div>'
  ).join('');
}

async function captureShot() {
  const cam = document.getElementById('cameraSelect').value;
  if (!cam) return;

  try {
    const resp = await fetch('/api/camera-snapshot/' + encodeURIComponent(cam) + '?t=' + Date.now());
    if (!resp.ok) throw new Error('ไม่สามารถดึงภาพจากกล้อง');
    const blob = await resp.blob();
    const b64 = await blobToBase64(blob);
    capturedImages.push(b64);
    advancePose();
  } catch (e) {
    showToast(e.message, 'error');
  }
}

function skipPose() {
  advancePose();
}

function advancePose() {
  currentPoseIdx++;
  renderPoseGuide();
  renderCapturedThumbs();

  if (currentPoseIdx >= POSES.length) {
    document.getElementById('captureBtn').disabled = true;
    document.getElementById('skipBtn').disabled = true;
  }

  // Allow registration if at least 1 image captured
  document.getElementById('camRegisterBtn').disabled = (capturedImages.length === 0);
}

async function registerFromCamera() {
  const name = document.getElementById('camPersonName').value.trim();
  if (!name) { showToast('กรุณาใส่ชื่อบุคคล', 'error'); return; }
  if (capturedImages.length === 0) { showToast('กรุณาถ่ายภาพอย่างน้อย 1 ภาพ', 'error'); return; }

  document.getElementById('camRegisterBtn').disabled = true;

  try {
    const resp = await fetch('/api/face/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, images: capturedImages })
    });
    const data = await resp.json();
    if (!resp.ok) throw new Error(data.detail || 'Registration failed');
    showToast('ลงทะเบียน "' + name + '" สำเร็จ (' + data.embeddings_count + ' embeddings จาก ' + capturedImages.length + ' ภาพ)', 'success');

    // Reset
    document.getElementById('camPersonName').value = '';
    capturedImages = [];
    currentPoseIdx = 0;
    renderPoseGuide();
    renderCapturedThumbs();
    document.getElementById('captureBtn').disabled = false;
    document.getElementById('skipBtn').disabled = false;
    document.getElementById('camRegisterBtn').disabled = true;
    loadGallery();
  } catch (e) {
    showToast('เกิดข้อผิดพลาด: ' + e.message, 'error');
    document.getElementById('camRegisterBtn').disabled = false;
  }
}

// --- Utilities ---
function fileToBase64(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result.split(',')[1]);
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
}

function blobToBase64(blob) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result.split(',')[1]);
    reader.onerror = reject;
    reader.readAsDataURL(blob);
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

// --- Detection History ---
async function loadDetectionHistory() {
  const container = document.getElementById('detectionHistory');
  try {
    const resp = await fetch('/api/events/identified');
    if (!resp.ok) throw new Error('Failed to load events');
    const data = await resp.json();

    const names = Object.keys(data);
    if (names.length === 0) {
      container.innerHTML = '<div class="empty-state"><p>ยังไม่มีการตรวจพบบุคคลที่ลงทะเบียน</p><p>เมื่อกล้องตรวจพบใบหน้าที่ตรงกับบุคคลที่ลงทะเบียน จะแสดงที่นี่อัตโนมัติ</p></div>';
      return;
    }

    let html = '';
    for (const name of names) {
      const events = data[name];
      if (!events || events.length === 0) continue;

      const lastEvent = events[0];
      const lastTime = new Date(lastEvent.start_time * 1000);
      const ago = timeAgo(lastEvent.start_time);

      html += '<div class="detection-person">';
      html += '<div class="detection-person-header">';
      html += '<span class="detection-person-name">' + escapeHtml(name) + '</span>';
      html += '<span class="detection-badge">ตรวจพบ</span>';
      html += '<span class="detection-person-count">' + events.length + ' ครั้ง</span>';
      html += '<span class="detection-person-last">ล่าสุด: ' + ago + '</span>';
      html += '</div>';
      html += '<div class="detection-grid">';

      for (const ev of events) {
        const t = new Date(ev.start_time * 1000);
        const ts = t.toLocaleTimeString('th-TH', { hour: '2-digit', minute: '2-digit' });
        const ds = t.toLocaleDateString('th-TH', { day: 'numeric', month: 'short' });
        const thumbSrc = ev.thumbnail
          ? 'data:image/jpeg;base64,' + ev.thumbnail
          : '/api/thumbnail/' + ev.id;
        const note = ev.note || '';
        const scoreMatch = note.match(/(\d+)%%/);
        const score = scoreMatch ? scoreMatch[1] + '%%' : '';

        html += '<div class="detection-card" onclick="window.open(\'/events\', \'_blank\')">';
        html += '<img src="' + thumbSrc + '" alt="' + escapeHtml(name) + '" loading="lazy">';
        html += '<div class="detection-card-info">';
        html += '<div class="detection-card-time">' + ds + ' ' + ts + '</div>';
        html += '<div class="detection-card-camera">' + escapeHtml(ev.camera) + '</div>';
        if (score) html += '<div class="detection-card-score">ความคล้าย: ' + score + '</div>';
        html += '</div></div>';
      }

      html += '</div></div>';
    }

    container.innerHTML = html;
  } catch (e) {
    container.innerHTML = '<div class="empty-state"><p>ไม่สามารถโหลดประวัติการตรวจพบ</p><p>' + escapeHtml(e.message) + '</p></div>';
  }
}

function timeAgo(timestamp) {
  const now = Date.now() / 1000;
  const diff = now - timestamp;
  if (diff < 60) return 'เมื่อสักครู่';
  if (diff < 3600) return Math.floor(diff / 60) + ' นาทีที่แล้ว';
  if (diff < 86400) return Math.floor(diff / 3600) + ' ชั่วโมงที่แล้ว';
  return Math.floor(diff / 86400) + ' วันที่แล้ว';
}

// Init
loadGallery();
loadDetectionHistory();
renderPoseGuide();

// Auto-refresh detection history every 30 seconds
setInterval(loadDetectionHistory, 30000);
</script>
` + i18nScript + `
</body>
</html>
`
