const { api, shortID, setMessage, withBusy } = window.LockFlow;
const form = document.querySelector("#plan-form");
const message = document.querySelector("#plan-message");
const vesselsHost = document.querySelector("#vessels");
const preview = document.querySelector("#stowage-preview");
const chamberSelect = document.querySelector("#chamber");
const stowageForm = document.querySelector("#stowage-form");
const stowageMessage = document.querySelector("#stowage-message");
let stowageByChamber = {};

const fmt = (value) => `${Number(value).toFixed(1)} m`;
const esc = (value) => String(value ?? "").replace(/[&<>"']/g, (ch) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[ch]));

function addVesselRow(name = "", callSign = "", length = "") {
  const row = document.createElement("div");
  row.className = "vessel-row";
  row.innerHTML = `
    <div><label>Vessel</label><input class="v-name" required></div>
    <div><label>Call sign</label><input class="v-call" required></div>
    <div><label>Length (m)</label><input class="v-len" type="number" min="1" step="0.5" required></div>
    <div><label>&nbsp;</label><button type="button" class="danger v-remove" title="Remove vessel">&times;</button></div>`;
  row.querySelector(".v-name").value = name;
  row.querySelector(".v-call").value = callSign;
  row.querySelector(".v-len").value = length;
  row.querySelector(".v-remove").addEventListener("click", () => { row.remove(); schedulePreview(); });
  row.addEventListener("input", schedulePreview);
  vesselsHost.appendChild(row);
  schedulePreview();
}

function collectVessels() {
  return [...vesselsHost.querySelectorAll(".vessel-row")]
    .map((row) => ({
      name: row.querySelector(".v-name").value.trim(),
      call_sign: row.querySelector(".v-call").value.trim(),
      length_m: Number(row.querySelector(".v-len").value),
    }))
    .filter((vessel) => vessel.name && vessel.call_sign && vessel.length_m > 0);
}

let previewTimer = 0;
function schedulePreview() {
  clearTimeout(previewTimer);
  previewTimer = setTimeout(refreshPreview, 250);
}

async function refreshPreview() {
  const vessels = collectVessels();
  if (!vessels.length) {
    preview.textContent = "";
    preview.className = "hint";
    return;
  }
  try {
    const result = await api("/api/stowage/preview", {
      method: "POST",
      body: JSON.stringify({ chamber_id: chamberSelect.value, vessels }),
    });
    const config = result.config;
    const unplaceable = result.unplaceable || [];
    if (unplaceable.length) {
      preview.className = "hint bad";
      preview.textContent = `${unplaceable.map((item) => item.vessel.name).join(", ")} exceed the effective length of ${fmt(config.effective_length_m)} and can never enter.`;
      return;
    }
    if (result.fits_single_lockage) {
      const group = (result.groups || [])[0];
      preview.className = "hint ok";
      preview.textContent = group
        ? `Fits one lockage: ${fmt(group.occupancy_m)} of ${fmt(config.effective_length_m)} used, ${fmt(group.remaining_m)} remaining.`
        : "";
      return;
    }
    const required = vessels.reduce((sum, vessel) => sum + vessel.length_m, 0) + config.safety_spacing_m * (vessels.length - 1);
    preview.className = "hint warn";
    preview.textContent = `Convoy needs ${result.groups.length} lockages (${fmt(required)} > ${fmt(config.effective_length_m)}) — creation will be rejected as a whole. Remove vessels or adjust the chamber stowage.`;
  } catch (error) {
    preview.className = "hint warn";
    preview.textContent = error.message;
  }
}

function renderStowage() {
  const status = stowageByChamber[chamberSelect.value];
  if (!status) return;
  document.querySelector("#stowage-effective").textContent = fmt(status.config.effective_length_m);
  document.querySelector("#stowage-spacing").textContent = fmt(status.config.safety_spacing_m);
  document.querySelector("#stowage-chamber-label").textContent = `Applies to ${chamberSelect.value}${status.default ? " (default values)" : ""}`;
  const next = status.next_group;
  document.querySelector("#stowage-next").textContent = next ? `${next.vessels.length} vessel(s)` : "Queue empty";
  const rows = (next?.vessels || []).map((item) =>
    `<tr><td>${esc(item.vessel.name)}</td><td>${esc(item.vessel.call_sign)}</td><td>${fmt(item.vessel.length_m)}</td><td>${fmt(item.occupancy_m)}</td></tr>`);
  document.querySelector("#next-group").innerHTML = rows.join("") || '<tr><td colspan="4" class="muted">No queued vessels</td></tr>';
}

function fillStowageInputs() {
  const status = stowageByChamber[chamberSelect.value];
  if (!status) return;
  document.querySelector("#cfg-effective").value = status.config.effective_length_m;
  document.querySelector("#cfg-spacing").value = status.config.safety_spacing_m;
}

async function refresh() {
  const [plans, chambers, health, stowage] = await Promise.all([
    api("/api/plans"), api("/api/chambers"), api("/healthz"), api("/api/stowage"),
  ]);
  stowageByChamber = stowage;
  document.querySelector("#health").textContent = health.status === "ok" ? "Devices online" : "Attention required";
  const occupancyByPlan = {};
  Object.values(stowage).forEach((status) => (status.plans || []).forEach((item) => { occupancyByPlan[item.plan_id] = item; }));
  const rows = Object.entries(plans).flatMap(([chamber, items]) => items.map((plan) => {
    const occupancy = occupancyByPlan[plan.id];
    const cell = occupancy ? `${fmt(occupancy.occupancy_m)}${occupancy.fits ? "" : " (exceeds config)"}` : "-";
    return `<tr><td>${chamber}</td><td>${shortID(plan.id)}</td><td>${plan.direction}</td><td>${plan.vessels.length}</td><td>${cell}</td><td>${new Date(plan.created_at).toLocaleTimeString()}</td></tr>`;
  }));
  document.querySelector("#plans").innerHTML = rows.join("") || '<tr><td colspan="6" class="muted">No active plans</td></tr>';
  const current = chambers.find((item) => item.cycle);
  document.querySelector("#metric-chamber").textContent = current?.chamber_id || "Idle";
  document.querySelector("#metric-stage").textContent = current?.cycle?.stage || "-";
  document.querySelector("#metric-cycle").textContent = shortID(current?.cycle?.id);
  renderStowage();
}

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  const vessels = collectVessels();
  if (!vessels.length) {
    setMessage(message, "Add at least one vessel with name, call sign and length.");
    return;
  }
  const button = form.querySelector('button[type="submit"]');
  await withBusy(button, async () => {
    try {
      const result = await api("/api/plans", { method: "POST", body: JSON.stringify({
        chamber_id: chamberSelect.value,
        direction: document.querySelector("#direction").value,
        vessels,
      }) });
      const summary = result.stowage;
      setMessage(message, `Plan ${shortID(result.plan.id)} admitted to ${result.plan.chamber_id} — stowage ${fmt(summary.occupancy_m)} of ${fmt(summary.effective_length_m)} (${fmt(summary.remaining_m)} remaining).`);
      await refresh();
    } catch (error) {
      const overflow = error.body?.overflow;
      if (overflow) {
        const excluded = (overflow.excluded || []).map((item) => `${item.vessel.name} (${fmt(item.occupancy_m)})`).join(", ");
        setMessage(message, `${error.message}\nRequired ${fmt(overflow.required_m)} vs effective ${fmt(overflow.effective_length_m)}. Excluded: ${excluded}.\nNo plan was created.`);
      } else {
        setMessage(message, error.message);
      }
    }
  });
});

stowageForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const button = stowageForm.querySelector('button[type="submit"]');
  await withBusy(button, async () => {
    try {
      await api(`/api/stowage/${chamberSelect.value}`, { method: "PUT", body: JSON.stringify({
        effective_length_m: Number(document.querySelector("#cfg-effective").value),
        safety_spacing_m: Number(document.querySelector("#cfg-spacing").value),
      }) });
      await refresh();
      fillStowageInputs();
      setMessage(stowageMessage, `Stowage saved for ${chamberSelect.value}.`);
      refreshPreview();
    } catch (error) { setMessage(stowageMessage, error.message); }
  });
});

document.querySelector("#add-vessel").addEventListener("click", () => addVesselRow());
chamberSelect.addEventListener("change", () => { fillStowageInputs(); renderStowage(); refreshPreview(); });

addVesselRow("River Star", "RS-204", 86);
refresh().then(fillStowageInputs).catch((error) => setMessage(message, error.message));
setInterval(() => refresh().catch(() => {}), 5000);
