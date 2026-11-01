// WaterStick Desktop Interface Logic — Restrained Systems Engineering

let selectedProfile = "balanced";
let activeDevice = "";

// Helper to safely call Wails Go methods or fallback to mock
async function callGo(method, ...args) {
    if (window.go && window.go.main && window.go.main.App && window.go.main.App[method]) {
        return await window.go.main.App[method](...args);
    }
    console.warn(`Wails method ${method} unavailable (browser preview mode)`);
    return null;
}

// -------------------------------------------------------------
// In-App Toast & Confirmation Modal (No browser alerts)
// -------------------------------------------------------------
function showToast(message, type = "info") {
    const container = document.getElementById('toast-container');
    if (!container) return;

    const toast = document.createElement('div');
    toast.className = `toast toast-${type}`;

    let badgeClass = "badge-info";
    let badgeText = "INFO";
    if (type === "success") { badgeClass = "badge-success"; badgeText = "SUCCÈS"; }
    if (type === "error") { badgeClass = "badge-danger"; badgeText = "ERREUR"; }
    if (type === "warning") { badgeClass = "badge-warning"; badgeText = "ATTENTION"; }

    toast.innerHTML = `<span class="badge ${badgeClass}">${badgeText}</span> <span>${message}</span>`;
    container.appendChild(toast);

    setTimeout(() => {
        toast.style.opacity = '0';
        toast.style.transform = 'translateY(6px)';
        setTimeout(() => toast.remove(), 250);
    }, 3800);
}

function showConfirm(title, desc) {
    return new Promise((resolve) => {
        const modal = document.getElementById('modal-confirm');
        const titleEl = document.getElementById('modal-title');
        const descEl = document.getElementById('modal-desc');
        const btnCancel = document.getElementById('modal-btn-cancel');
        const btnConfirm = document.getElementById('modal-btn-confirm');

        titleEl.textContent = title;
        descEl.textContent = desc;
        modal.classList.add('show');

        const onConfirm = () => {
            modal.classList.remove('show');
            cleanup();
            resolve(true);
        };

        const onCancel = () => {
            modal.classList.remove('show');
            cleanup();
            resolve(false);
        };

        const cleanup = () => {
            btnConfirm.removeEventListener('click', onConfirm);
            btnCancel.removeEventListener('click', onCancel);
        };

        btnConfirm.addEventListener('click', onConfirm);
        btnCancel.addEventListener('click', onCancel);
    });
}

// -------------------------------------------------------------
// Navigation Tab Switching
// -------------------------------------------------------------
document.querySelectorAll('.nav-btn').forEach(btn => {
    btn.addEventListener('click', () => {
        const targetTab = btn.getAttribute('data-tab');

        document.querySelectorAll('.nav-btn').forEach(b => b.classList.remove('active'));
        document.querySelectorAll('.tab-panel').forEach(p => p.classList.remove('active'));

        btn.classList.add('active');
        const panel = document.getElementById(targetTab);
        if (panel) panel.classList.add('active');

        const titles = {
            'tab-health': 'Diagnostics Matériels & OS',
            'tab-debloat': 'Profils d\'Optimisation Système',
            'tab-rollback': 'Restauration de la Configuration',
            'tab-sideload': 'Catalogue d\'Applications Tiers',
            'tab-connect': 'Connexion au Périphérique'
        };
        document.getElementById('page-title').textContent = titles[targetTab] || 'WaterStick';

        if (targetTab === 'tab-health') loadDiagnostics();
        if (targetTab === 'tab-debloat') loadTweaks();
        if (targetTab === 'tab-sideload') loadApps();
    });
});

// Profile Selection
document.querySelectorAll('.profile-card').forEach(card => {
    card.addEventListener('click', () => {
        document.querySelectorAll('.profile-card').forEach(c => c.classList.remove('selected'));
        card.classList.add('selected');
        selectedProfile = card.getAttribute('data-profile');
    });
});

// -------------------------------------------------------------
// Connect Action
// -------------------------------------------------------------
document.getElementById('btn-connect').addEventListener('click', async () => {
    const ip = document.getElementById('ip-input').value.trim();
    const btn = document.getElementById('btn-connect');

    btn.disabled = true;
    btn.textContent = 'Connexion...';

    try {
        const res = await callGo('Connect', ip);
        activeDevice = ip;
        updateDeviceStatus(true, ip);
        showToast(res || `Connexion établie avec ${ip}`, 'success');
        document.querySelector('[data-tab="tab-health"]').click();
    } catch (err) {
        showToast('Erreur de connexion : ' + err, 'error');
    } finally {
        btn.disabled = false;
        btn.textContent = 'Connecter';
    }
});

// -------------------------------------------------------------
// Load Diagnostics
// -------------------------------------------------------------
async function loadDiagnostics() {
    try {
        const info = await callGo('InspectDevice');
        if (!info) return;

        document.getElementById('stat-model').textContent = info.model_name || info.model;
        const sub = document.getElementById('stat-model-sub');
        if (info.is_fire_tv_4k_max_2gen) {
            sub.textContent = `${info.model} • Karat (2nd Gen)`;
        } else {
            sub.textContent = `${info.model} • Périphérique Fire TV`;
        }

        document.getElementById('stat-os').textContent = info.fire_os_version;
        document.getElementById('stat-android').textContent = `Android ${info.android_version} Base (API 30)`;
        document.getElementById('stat-storage').textContent = `${info.free_storage} libres / ${info.total_storage}`;
        document.getElementById('stat-ram').textContent = `${info.free_ram_mb} Mo libres / ${info.total_ram_mb} Mo`;
        document.getElementById('status-disabled-count').textContent = `${info.disabled_count} paquets`;

        // Progress bars
        const storageFill = document.getElementById('storage-fill');
        if (storageFill && info.used_storage_pct) {
            storageFill.style.width = info.used_storage_pct;
        }

        const ramFill = document.getElementById('ram-fill');
        if (ramFill && info.total_ram_mb > 0) {
            const usedPct = ((info.total_ram_mb - info.free_ram_mb) / info.total_ram_mb) * 100;
            ramFill.style.width = `${Math.min(Math.max(usedPct, 8), 100)}%`;
        }

        // SoC Thermal Sensors
        const tempEl = document.getElementById('stat-temp');
        const thermalBadge = document.getElementById('stat-thermal-badge');
        if (info.cpu_temp_c > 0) {
            tempEl.textContent = `${info.cpu_temp_c.toFixed(1)} °C CPU • ${info.gpu_temp_c.toFixed(1)} °C GPU`;
            if (info.thermal_status === 0) {
                thermalBadge.textContent = 'Nominal (0 Throttling)';
                thermalBadge.className = 'badge badge-success';
            } else if (info.thermal_status === 1) {
                thermalBadge.textContent = 'Throttling Léger';
                thermalBadge.className = 'badge badge-warning';
            } else {
                thermalBadge.textContent = 'Throttling Sévère';
                thermalBadge.className = 'badge badge-danger';
            }
        }

        // CPU Clocks & Load
        const cpuFreqEl = document.getElementById('stat-cpu-freq');
        const cpuLoadEl = document.getElementById('stat-cpu-load');
        if (info.cpu_freq_mhz && info.cpu_freq_mhz.length > 0) {
            cpuFreqEl.textContent = `${info.cpu_freq_mhz[0]} MHz (4 Cœurs)`;
        }
        if (info.cpu_load) {
            cpuLoadEl.textContent = `Charge: ${info.cpu_load}`;
        }

        // OTA Shield status
        const otaBadge = document.getElementById('status-ota');
        const otaToggleLabel = document.getElementById('label-ota-toggle');
        if (info.ota_blocked) {
            otaBadge.textContent = 'Gelé (Bloqué)';
            otaBadge.className = 'badge badge-success';
            if (otaToggleLabel) otaToggleLabel.textContent = 'Dégeler le Bouclier OTA';
        } else {
            otaBadge.textContent = 'Actif (Non bloqué)';
            otaBadge.className = 'badge badge-warning';
            if (otaToggleLabel) otaToggleLabel.textContent = 'Activer le Bouclier OTA';
        }

        // ZRAM
        const zramBadge = document.getElementById('status-zram');
        if (zramBadge) {
            zramBadge.textContent = `${info.zram_used_mb || 0} Mo compressés`;
        }

        // Load Live Packages
        await loadPackages();
    } catch (err) {
        console.error('Failed to load diagnostics:', err);
    }
}

// -------------------------------------------------------------
// Quick Actions
// -------------------------------------------------------------
document.getElementById('btn-purge-ram').addEventListener('click', async () => {
    const btn = document.getElementById('btn-purge-ram');
    const status = document.getElementById('quick-action-status');
    btn.disabled = true;
    status.textContent = 'Purge en cours...';
    try {
        const res = await callGo('PurgeRAM');
        status.textContent = res || 'Mémoire purgée';
        showToast(res || 'Cache mémoire et tampons vidéo purgés avec succès', 'success');
        await loadDiagnostics();
    } catch (err) {
        status.textContent = 'Erreur: ' + err;
        showToast('Erreur lors de la purge : ' + err, 'error');
    } finally {
        btn.disabled = false;
        setTimeout(() => { status.textContent = ''; }, 4000);
    }
});

document.getElementById('btn-toggle-ota').addEventListener('click', async () => {
    const btn = document.getElementById('btn-toggle-ota');
    const status = document.getElementById('quick-action-status');
    btn.disabled = true;
    status.textContent = 'Mise à jour du bouclier...';
    try {
        const res = await callGo('ToggleOTA');
        status.textContent = res || 'État OTA modifié';
        showToast(res || 'État du Bouclier OTA modifié avec succès', 'success');
        await loadDiagnostics();
    } catch (err) {
        status.textContent = 'Erreur: ' + err;
        showToast('Erreur OTA : ' + err, 'error');
    } finally {
        btn.disabled = false;
        setTimeout(() => { status.textContent = ''; }, 4000);
    }
});

// -------------------------------------------------------------
// Apply Profile
// -------------------------------------------------------------
document.getElementById('btn-apply-profile').addEventListener('click', async () => {
    const btn = document.getElementById('btn-apply-profile');
    const status = document.getElementById('apply-status');

    btn.disabled = true;
    status.textContent = 'Application en cours...';

    try {
        const report = await callGo('ApplyProfile', selectedProfile);
        status.textContent = `Appliqué (${report ? report.disabled_count : 0} paquets désactivés)`;
        showToast(`Profil appliqué avec succès (${report ? report.disabled_count : 0} paquets désactivés). Voix Alexa intacte.`, 'success');
        await loadDiagnostics();
    } catch (err) {
        status.textContent = 'Erreur: ' + err;
        showToast('Erreur lors de l\'optimisation : ' + err, 'error');
    } finally {
        btn.disabled = false;
        setTimeout(() => { status.textContent = ''; }, 4000);
    }
});

// -------------------------------------------------------------
// Rollback All with Custom Modal
// -------------------------------------------------------------
document.getElementById('btn-rollback').addEventListener('click', async () => {
    const confirmed = await showConfirm(
        'Confirmer la Restauration',
        'Cette opération réactivera tous les composants système Amazon précédemment désactivés et rétablira les réglages Wi-Fi et d\'animation d\'origine.'
    );
    if (!confirmed) return;

    const btn = document.getElementById('btn-rollback');
    const status = document.getElementById('rollback-status');

    btn.disabled = true;
    status.textContent = 'Restauration en cours...';

    try {
        const res = await callGo('RollbackAll');
        status.textContent = res || 'Restauration terminée';
        showToast(res || 'Tous les paquets et paramètres d\'usine ont été restaurés.', 'success');
        await loadDiagnostics();
    } catch (err) {
        status.textContent = 'Erreur: ' + err;
        showToast('Erreur lors de la restauration : ' + err, 'error');
    } finally {
        btn.disabled = false;
        setTimeout(() => { status.textContent = ''; }, 4000);
    }
});

// -------------------------------------------------------------
// Load Sideload Apps Data Table
// -------------------------------------------------------------
async function loadApps() {
    const container = document.getElementById('apps-container');
    container.innerHTML = '<div style="padding: 20px; font-size: 13px; color: var(--text-secondary);">Chargement du catalogue...</div>';

    try {
        const apps = await callGo('GetApps') || [
            { id: "smarttube", name: "SmartTube", description: "Lecteur YouTube sans publicité avec SponsorBlock et décodage 4K 60fps", installed: false },
            { id: "stremio", name: "Stremio for Android TV", description: "Agrégateur moderne de streaming (films, séries) avec extensions", installed: false },
            { id: "kodi", name: "Kodi", description: "Centre multimédia open-source ultra-complet pour bibliothèques locales et distantes", installed: false },
            { id: "nova", name: "Nova Video Player", description: "Lecteur vidéo Android TV open-source avec indexation automatique", installed: false },
            { id: "vlc", name: "VLC Media Player", description: "Lecteur universel tout-terrain compatible avec tous les codecs", installed: false },
            { id: "jellyfin", name: "Jellyfin Android TV", description: "Client officiel 100% libre pour streamer votre serveur Jellyfin", installed: false }
        ];

        container.innerHTML = '';
        apps.forEach(app => {
            const row = document.createElement('div');
            row.className = 'app-row';
            row.innerHTML = `
                <div class="app-info">
                    <h4>${app.name}</h4>
                    <p>${app.description}</p>
                </div>
                <div style="display: flex; align-items: center; gap: 12px;">
                    ${app.installed ? '<span class="badge badge-success">Installé</span>' : '<span class="badge badge-neutral">Disponible</span>'}
                    <button class="btn btn-secondary" onclick="installApp('${app.id}', this)">
                        ${app.installed ? 'Mettre à jour' : 'Installer'}
                    </button>
                </div>
            `;
            container.appendChild(row);
        });
    } catch (err) {
        container.innerHTML = '<div style="padding: 20px; font-size: 13px; color: var(--status-danger);">Erreur lors du chargement des applications.</div>';
    }
}

window.installApp = async function(appID, btn) {
    btn.disabled = true;
    btn.textContent = 'Installation...';
    try {
        const res = await callGo('InstallApp', appID);
        showToast(res || `Application ${appID} installée avec succès`, 'success');
        btn.textContent = 'Installé';
        await loadApps();
    } catch (err) {
        showToast('Erreur d\'installation : ' + err, 'error');
        btn.disabled = false;
        btn.textContent = 'Installer';
    }
};

// -------------------------------------------------------------
// Disconnect Action
// -------------------------------------------------------------
const btnDisco = document.getElementById('btn-disconnect');
if (btnDisco) {
    btnDisco.addEventListener('click', async () => {
        btnDisco.disabled = true;
        btnDisco.textContent = 'Déconnexion...';
        try {
            const res = await callGo('Disconnect');
            activeDevice = '';
            updateDeviceStatus(false, 'Non connecté');
            showToast(res || 'Périphérique déconnecté', 'info');
        } catch (err) {
            showToast('Erreur de déconnexion : ' + err, 'error');
        } finally {
            btnDisco.disabled = false;
            btnDisco.textContent = 'Déconnecter';
        }
    });
}

// -------------------------------------------------------------
// Package Explorer
// -------------------------------------------------------------
let allPackages = [];
let currentPkgFilter = 'all';

async function loadPackages() {
    const tbody = document.getElementById('packages-tbody');
    const summary = document.getElementById('packages-summary');
    if (!tbody) return;

    try {
        allPackages = await callGo('GetPackages') || [];
        renderPackagesTable();
    } catch (err) {
        tbody.innerHTML = `<tr><td colspan="4" style="color: var(--status-danger); padding: 12px;">Erreur de chargement des paquets : ${err}</td></tr>`;
    }
}

window.setPackageFilter = function(filter) {
    currentPkgFilter = filter;
    document.querySelectorAll('#btn-filter-all, #btn-filter-disabled, #btn-filter-active').forEach(b => b.classList.remove('active'));
    const target = document.getElementById(`btn-filter-${filter}`);
    if (target) target.classList.add('active');
    renderPackagesTable();
};

function renderPackagesTable() {
    const tbody = document.getElementById('packages-tbody');
    const summary = document.getElementById('packages-summary');
    if (!tbody) return;

    let filtered = allPackages;
    if (currentPkgFilter === 'disabled') {
        filtered = allPackages.filter(p => p.disabled);
    } else if (currentPkgFilter === 'active') {
        filtered = allPackages.filter(p => !p.disabled);
    }

    const disabledCount = allPackages.filter(p => p.disabled).length;
    if (summary) {
        summary.textContent = `${allPackages.length} paquets managés (${disabledCount} gelés, ${allPackages.length - disabledCount} actifs) • Voix Alexa 100% protégée`;
    }

    if (filtered.length === 0) {
        tbody.innerHTML = `<tr><td colspan="4" style="color: var(--text-muted); padding: 12px;">Aucun paquet correspondant au filtre.</td></tr>`;
        return;
    }

    tbody.innerHTML = '';
    filtered.forEach(p => {
        const tr = document.createElement('tr');
        const isVoice = p.tier === 'tier_voice';
        const isHardLocked = p.tier === 'tier_0_hardlocked';

        let badgeStatus = p.disabled
            ? '<span class="badge badge-warning">GELÉ</span>'
            : '<span class="badge badge-success">ACTIF</span>';

        let badgeRole = isVoice
            ? '<span class="badge badge-info">Voix Alexa (Sanctuarisé)</span>'
            : (isHardLocked ? '<span class="badge badge-neutral">Système Vital</span>' : `<span style="color: var(--text-muted); font-size: 11px;">${p.category || p.tier}</span>`);

        let actionBtn = '';
        if (isVoice || isHardLocked) {
            actionBtn = '<span style="color: var(--text-muted); font-size: 11px;">Intouchable</span>';
        } else if (p.disabled) {
            actionBtn = `<button class="btn btn-secondary btn-sm" onclick="togglePackage('${p.id}', this)">Réactiver</button>`;
        } else {
            actionBtn = `<button class="btn btn-secondary btn-sm" onclick="togglePackage('${p.id}', this)">Geler</button>`;
        }

        tr.innerHTML = `
            <td style="font-family: monospace; font-size: 12px; color: var(--text-primary);">${p.id}</td>
            <td>${badgeStatus}</td>
            <td>${badgeRole}</td>
            <td style="text-align: right;">${actionBtn}</td>
        `;
        tbody.appendChild(tr);
    });
}

window.togglePackage = async function(pkgID, btn) {
    btn.disabled = true;
    btn.textContent = '...';
    try {
        const res = await callGo('TogglePackage', pkgID);
        showToast(res, 'success');
        await loadPackages();
        await loadDiagnostics();
    } catch (err) {
        showToast('Action bloquée : ' + err, 'error');
        btn.disabled = false;
    }
};

function updateDeviceStatus(online, label) {
    const dot = document.getElementById('status-dot');
    const txt = document.getElementById('device-label');
    if (online) {
        dot.className = 'status-dot online';
        txt.textContent = label;
    } else {
        dot.className = 'status-dot offline';
        txt.textContent = 'Non connecté';
    }
}

// Auto-check connected device on startup
window.addEventListener('DOMContentLoaded', async () => {
    try {
        const dev = await callGo('GetConnectedDevice');
        if (dev) {
            activeDevice = dev;
            updateDeviceStatus(true, dev);
            await loadDiagnostics();
            await loadTweaks();
        }
    } catch (e) {
        console.log('No active device yet');
    }

    // Check for application updates in the background
    checkForAppUpdates();
});

// -------------------------------------------------------------
// Advanced Hardware Tweaks Logic
// -------------------------------------------------------------
let tweakData = null;

async function loadTweaks() {
    try {
        const data = await callGo('GetTweakStatus');
        if (!data) return;
        tweakData = data;
        updateTweakUI();
    } catch (e) {
        console.warn('Failed to load tweak status', e);
    }
}

function updateTweakUI() {
    if (!tweakData) return;

    // 1. Optical 5.1
    const btnOpt = document.getElementById('btn-toggle-optical');
    if (btnOpt && tweakData.audio) {
        if (tweakData.audio.is_optical_51_optimized) {
            btnOpt.className = 'btn btn-primary btn-sm';
            btnOpt.textContent = 'ACTIVÉ (AC-3 5.1 Bitstream)';
        } else {
            btnOpt.className = 'btn btn-secondary btn-sm';
            btnOpt.textContent = 'Auto / HDMI Standard';
        }
    }

    // 2. Navigation Sounds
    const btnNav = document.getElementById('btn-toggle-navsounds');
    if (btnNav && tweakData.audio) {
        if (!tweakData.audio.navigation_sounds) {
            btnNav.className = 'btn btn-primary btn-sm';
            btnNav.textContent = 'Silencieux (Coupés)';
        } else {
            btnNav.className = 'btn btn-secondary btn-sm';
            btnNav.textContent = 'Activés (Standard)';
        }
    }

    // 3. Match Frame Rate 24p
    const btnMfr = document.getElementById('btn-toggle-matchframerate');
    if (btnMfr && tweakData.display) {
        if (tweakData.display.match_frame_rate) {
            btnMfr.className = 'btn btn-primary btn-sm';
            btnMfr.textContent = 'ACTIVÉ (24p sans saccades)';
        } else {
            btnMfr.className = 'btn btn-secondary btn-sm';
            btnMfr.textContent = 'Désactivé (60Hz fixe)';
        }
    }

    // 4. Ambient Experience
    const btnAmb = document.getElementById('btn-toggle-ambient');
    if (btnAmb && tweakData.power) {
        if (!tweakData.power.ambient_experience_enabled) {
            btnAmb.className = 'btn btn-primary btn-sm';
            btnAmb.textContent = 'DÉSACTIVÉ (Veille coupée)';
        } else {
            btnAmb.className = 'btn btn-secondary btn-sm';
            btnAmb.textContent = 'Actif (Standard)';
        }
    }

    // 5. DoT DNS Select
    const selDns = document.getElementById('select-dns');
    if (selDns && tweakData.dns) {
        if (tweakData.dns.mode === 'off') {
            selDns.value = 'off';
        } else if (tweakData.dns.specifier && tweakData.dns.specifier.includes('adguard') && tweakData.dns.specifier.includes('family')) {
            selDns.value = 'family';
        } else if (tweakData.dns.specifier && tweakData.dns.specifier.includes('adguard')) {
            selDns.value = 'adguard';
        } else if (tweakData.dns.specifier && tweakData.dns.specifier.includes('quad9')) {
            selDns.value = 'quad9';
        } else if (tweakData.dns.specifier && (tweakData.dns.specifier.includes('one.one.one.one') || tweakData.dns.specifier.includes('cloudflare'))) {
            selDns.value = 'cloudflare';
        }
    }
}

window.toggleOptical51 = async function() {
    const isCur = tweakData && tweakData.audio && tweakData.audio.is_optical_51_optimized;
    const nextState = !isCur;
    try {
        const res = await callGo('SetOptical51', nextState);
        showToast(res, 'success');
        await loadTweaks();
    } catch (e) {
        showToast('Erreur audio : ' + e, 'error');
    }
};

window.toggleNavSounds = async function() {
    const isCur = tweakData && tweakData.audio && tweakData.audio.navigation_sounds;
    const nextState = !isCur;
    try {
        const res = await callGo('SetNavSounds', nextState);
        showToast(res, 'success');
        await loadTweaks();
    } catch (e) {
        showToast('Erreur : ' + e, 'error');
    }
};

window.toggleMatchFrameRate = async function() {
    const isCur = tweakData && tweakData.display && tweakData.display.match_frame_rate;
    const nextState = !isCur;
    try {
        const res = await callGo('SetMatchFrameRate', nextState);
        showToast(res, 'success');
        await loadTweaks();
    } catch (e) {
        showToast('Erreur fluidité : ' + e, 'error');
    }
};

window.toggleAmbient = async function() {
    const isCur = tweakData && tweakData.power && tweakData.power.ambient_experience_enabled;
    const nextState = !isCur;
    try {
        const res = await callGo('SetAmbient', nextState);
        showToast(res, 'success');
        await loadTweaks();
    } catch (e) {
        showToast('Erreur ambient : ' + e, 'error');
    }
};

window.changeDNS = async function(val) {
    try {
        const res = await callGo('SetDNS', val);
        showToast(res, 'success');
        await loadTweaks();
    } catch (e) {
        showToast('Erreur DNS : ' + e, 'error');
    }
};

async function checkForAppUpdates() {
    try {
        const res = await callGo('CheckUpdate');
        if (res && res.available) {
            const badge = document.getElementById('update-badge');
            const txt = document.getElementById('update-text');
            const btn = document.getElementById('btn-apply-update');
            if (badge && txt && btn) {
                txt.textContent = 'Mise à jour ' + res.latest_version + ' disponible';
                badge.style.display = 'flex';
                btn.onclick = async function() {
                    btn.disabled = true;
                    btn.textContent = 'Installation...';
                    showToast('Téléchargement et installation de la mise à jour...', 'info');
                    try {
                        const msg = await callGo('ApplyUpdate');
                        showToast(msg, 'success');
                        txt.textContent = 'Mise à jour terminée';
                        btn.textContent = 'Terminé';
                    } catch (err) {
                        showToast('Erreur mise à jour : ' + err, 'error');
                        btn.disabled = false;
                        btn.textContent = 'Réessayer';
                    }
                };
            }
        }
    } catch (e) {
        // Silent catch for background check
    }
}

