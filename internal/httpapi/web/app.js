const $ = (selector, root = document) => root.querySelector(selector);
const $$ = (selector, root = document) => [...root.querySelectorAll(selector)];
const state = { batches: [], detail: null, selected: null, tab: "register", planDigest: "", filters: {}, remediationFilters: {} };
const labels = { draft: "登记中", submitted: "已送检", inspecting: "检查中", ready: "待批准", approved: "已准用" };
const actionLabels = { "batch.created": "创建检查批次", "batch.updated": "更新批次信息", "unit.registered": "登记布景单元", "unit.updated": "更新布景登记", "unit.withdrawn": "撤回布景登记", "plan.submitted": "冻结方案并送检", "check.recorded": "记录检查结果", "check.batch_recorded": "批量记录检查结果", "remediation.opened": "发起缺陷整改", "remediation.changed": "调整整改责任与期限", "remediation.completed": "完成整改并提交证据", "batch.approved": "批准并签发准用凭据" };

function escapeHTML(value) {
  return String(value ?? "").replace(/[&<>'"]/g, char => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", '"': "&quot;" })[char]);
}
function key(prefix) { return `${prefix}-${Date.now()}-${crypto.randomUUID()}`; }
function toast(message, error = false) {
  const node = $("#toast"); node.textContent = message; node.className = error ? "show error" : "show";
  setTimeout(() => node.className = "", 2600);
}
async function request(path, options = {}) {
  const response = await fetch(path, { headers: options.body ? { "Content-Type": "application/json" } : {}, ...options });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(data.error?.message || `请求失败 (${response.status})`);
  return data;
}
async function loadBatches(selectNewest = false) {
  const data = await request("/api/v1/batches"); state.batches = data.items || []; renderBatchList();
  if (selectNewest && state.batches.length) await selectBatch(state.batches[0].id);
}
function renderBatchList() {
  const list = $("#batch-list");
  list.innerHTML = state.batches.length ? state.batches.map(batch => `<button class="batch-item ${batch.id === state.selected ? "active" : ""}" data-batch="${escapeHTML(batch.id)}"><strong>${escapeHTML(batch.title)}</strong><span>${escapeHTML(labels[batch.state] || batch.state)} · r${batch.revision}</span><span>${escapeHTML(batch.venue)}</span></button>`).join("") : `<p class="mini">尚无检查批次</p>`;
  $$('[data-batch]', list).forEach(node => node.addEventListener("click", () => selectBatch(node.dataset.batch)));
}
function detailPath(id) {
  const values = { ...state.filters, ...state.remediationFilters }, query = new URLSearchParams();
  Object.entries(values).forEach(([name, value]) => { if (value) query.set(name, value); });
  return `/api/v1/batches/${encodeURIComponent(id)}${query.size ? `?${query}` : ""}`;
}
async function selectBatch(id) {
  state.selected = id; state.planDigest = ""; state.detail = await request(detailPath(id));
  $("#empty-state").classList.add("hidden"); $("#batch-view").classList.remove("hidden");
  renderBatchList(); renderDetail();
}
async function refresh() { if (state.selected) { state.detail = await request(detailPath(state.selected)); await loadBatches(); renderDetail(); } }
function renderDetail() {
  const detail = state.detail, aggregate = detail.aggregate, batch = aggregate.batch;
  $("#batch-title").textContent = batch.title; $("#batch-state").textContent = labels[batch.state] || batch.state;
  $("#batch-meta").textContent = `${batch.venue} · ${new Date(batch.performanceAt).toLocaleString("zh-CN")} · 负责人 ${batch.coordinator}`;
  $("#revision").textContent = batch.revision;
  $("#edit-batch").classList.toggle("hidden", batch.state !== "draft");
  renderUnits(aggregate); renderPlan(aggregate); renderMatrix(detail); renderRemediations(detail); renderPermit(detail); setTab(state.tab);
}
function renderUnits(aggregate) {
  $("#show-unit").classList.toggle("hidden", aggregate.batch.state !== "draft");
  $("#unit-cards").innerHTML = aggregate.units.length ? aggregate.units.map(unit => `<article class="unit-card"><h4>${escapeHTML(unit.unitCode)} · ${escapeHTML(unit.name)}</h4><p>${escapeHTML(unit.stageZone)} / ${escapeHTML(unit.materialClass)}</p><p>供应：${escapeHTML(unit.supplier)}</p><p>处理批次：${escapeHTML(unit.treatmentLot)}</p><p>证据 ${unit.evidenceRefs.length} 项</p>${aggregate.batch.state === "draft" ? `<div class="unit-tools"><button class="quiet" data-edit-unit="${escapeHTML(unit.id)}">编辑</button><button class="quiet" data-remove-unit="${escapeHTML(unit.id)}">撤回</button></div>` : ""}</article>`).join("") : `<p class="mini">尚未登记布景单元。</p>`;
  $$('[data-edit-unit]').forEach(button => button.addEventListener("click", () => editUnit(button.dataset.editUnit)));
  $$('[data-remove-unit]').forEach(button => button.addEventListener("click", () => removeUnit(button.dataset.removeUnit)));
}
function definitionRow(definition = {}) {
  const wrapper = document.createElement("div"); wrapper.className = "definition-row";
  wrapper.innerHTML = `<input name="code" required placeholder="检查编号" value="${escapeHTML(definition.code || "")}"><input name="name" required placeholder="检查项目" value="${escapeHTML(definition.name || "")}"><input name="criterion" required placeholder="判定标准" value="${escapeHTML(definition.criterion || "")}"><label><input name="blocking" type="checkbox" ${definition.blocking !== false ? "checked" : ""}> 阻断项</label><button type="button" class="icon-button remove">×</button>`;
  $(".remove", wrapper).addEventListener("click", () => { wrapper.remove(); invalidatePreview(); }); return wrapper;
}
function invalidatePreview() { state.planDigest=""; $("#submit-plan").disabled=true; $("#preflight-result").className="preflight mini"; $("#preflight-result").textContent="方案或登记已变化，请重新预检。"; }
function renderPlan(aggregate) {
  const form = $("#plan-form"); form.classList.toggle("hidden", aggregate.batch.state !== "draft");
  if (aggregate.batch.state === "draft" && !$("#definition-list").children.length) {
    $("#definition-list").append(definitionRow({ code: "SURFACE-FLAME", name: "表面续燃检查", criterion: "移除火源后无持续明火", blocking: true }));
    $("#definition-list").append(definitionRow({ code: "CERT-TRACE", name: "阻燃凭证追溯", criterion: "材料与处理批次凭证一致", blocking: true }));
  }
	if (aggregate.batch.state !== "draft") state.planDigest = "";
}
function planDefinitions(root = $("#plan-form")) { return $$(".definition-row",root).map(row=>({code:$('[name="code"]',row).value,name:$('[name="name"]',row).value,criterion:$('[name="criterion"]',row).value,required:true,blocking:$('[name="blocking"]',row).checked})); }
function unitName(id) { return state.detail.aggregate.units.find(unit => unit.id === id)?.name || id; }
function renderMatrix(detail) {
  const matrix = detail.matrix || [], passed = matrix.filter(cell => cell.status === "pass").length, failed = matrix.filter(cell => cell.status === "fail").length;
  $("#matrix-summary").textContent = `${passed}/${matrix.length} 合格 · ${failed} 不合格`;
  $("#progress-groups").innerHTML = (detail.progress?.groups || []).map(group => `<button type="button" class="progress-card" data-risk-zone="${escapeHTML(group.stageZone)}" data-risk-material="${escapeHTML(group.materialClass)}"><strong>${escapeHTML(group.stageZone)} · ${escapeHTML(group.materialClass)}</strong><span>${Math.round(group.completion*100)}% 完成</span><span>待检 ${group.pending} / 合格 ${group.passed} / 不合格 ${group.failed} / 阻断 ${group.blocking}</span></button>`).join("");
  $("#matrix-body").innerHTML = matrix.length ? matrix.map(cell => `<tr><td>${canRecord(cell) ? `<input type="checkbox" data-select-cell data-unit="${escapeHTML(cell.unitId)}" data-check="${escapeHTML(cell.checkCode)}">` : "—"}</td><td><strong>${escapeHTML(unitName(cell.unitId))}</strong></td><td>${escapeHTML(cell.definition.name)}<br><small>${escapeHTML(cell.definition.criterion)}</small></td><td><span class="pill ${cell.status}">${cell.status === "pass" ? "合格" : cell.status === "fail" ? "不合格" : "待检"}</span></td><td>${cell.latest?.attempt || "—"}</td><td>${escapeHTML(cell.latest?.inspector || "—")}</td><td>${matrixAction(cell)}</td></tr>`).join("") : `<tr><td colspan="7">送检后生成检查矩阵。</td></tr>`;
  $$('[data-check]', $("#matrix-body")).forEach(button => button.addEventListener("click", () => openResult(button.dataset.unit, button.dataset.check)));
  $$('[data-remediate]', $("#matrix-body")).forEach(button => button.addEventListener("click", () => openRemediation(button.dataset.remediate)));
  $$('[data-risk-zone]').forEach(button => button.addEventListener("click", async () => { state.filters={stageZone:button.dataset.riskZone,materialClass:button.dataset.riskMaterial,status:"blocking"}; $("#progress-filter").elements.stageZone.value=button.dataset.riskZone; $("#progress-filter").elements.materialClass.value=button.dataset.riskMaterial; $("#progress-filter").elements.status.value="blocking"; await refresh(); }));
}
function canRecord(cell) { return cell.status === "pending" || (cell.status === "fail" && cell.remediation?.status === "completed"); }
function matrixAction(cell) {
  if (cell.status === "pending") return `<button class="secondary" data-check="${escapeHTML(cell.checkCode)}" data-unit="${escapeHTML(cell.unitId)}">记录</button>`;
  if (cell.status === "fail" && !cell.remediation) return `<button class="secondary" data-remediate="${escapeHTML(cell.latest.id)}">发起整改</button>`;
  if (cell.status === "fail" && cell.remediation?.status === "completed") return `<button class="primary" data-check="${escapeHTML(cell.checkCode)}" data-unit="${escapeHTML(cell.unitId)}">发起复测</button>`;
  return "—";
}
function renderRemediations(detail) {
  const items = detail.remediationQueue || [];
  $("#remediation-list").innerHTML = items.length ? items.map(item => { const rem=item.remediation, risk={normal:"正常",due_soon:"临期",overdue:"逾期"}[item.dueRisk]; return `<article class="queue-card"><h4>${escapeHTML(unitName(item.unitId))} · ${escapeHTML(item.checkCode)}</h4><p>责任人 ${escapeHTML(rem.owner)} · 截止 ${new Date(rem.dueAt).toLocaleString("zh-CN")} · <span class="risk-${item.dueRisk}">${risk}</span></p><span class="pill ${rem.status === "closed" ? "pass" : "fail"}">${{open:"待整改",completed:"待复测",closed:"已闭环"}[rem.status]}</span>${rem.status === "open" ? `<button class="quiet" data-change-rem="${escapeHTML(rem.id)}">调整责任与期限</button><button class="secondary" data-complete="${escapeHTML(rem.id)}">登记整改完成</button>` : ""}${rem.completedOverdue ? `<p>完成时逾期 ${Math.ceil(rem.overdueSeconds/3600)} 小时</p>` : ""}${rem.actionNote ? `<p>整改说明：${escapeHTML(rem.actionNote)}</p>` : ""}</article>`; }).join("") : `<p class="mini">当前筛选下没有整改记录。</p>`;
  $$('[data-complete]').forEach(button => button.addEventListener("click", () => completeRemediation(button.dataset.complete)));
  $$('[data-change-rem]').forEach(button => button.addEventListener("click", () => changeRemediation(button.dataset.changeRem)));
}
function renderPermit(detail) {
  const aggregate = detail.aggregate, permit = aggregate.permit;
  if (aggregate.batch.state === "ready") $("#approval-box").innerHTML = `<h4>所有必检项均已合格，整改已闭环</h4><p>批准将原子冻结 ${aggregate.units.length} 个准许上台布景单元并签发不可变凭据。</p><button id="approve" class="primary warning">安全负责人批准并签发</button>`;
  else if (aggregate.batch.state === "approved") $("#approval-box").innerHTML = `<strong>本批次已批准，冻结数据不可修改。</strong>`;
  else $("#approval-box").innerHTML = `<strong>尚不可批准</strong><p>请完成全部必检项并关闭所有整改。</p>`;
  $("#approve")?.addEventListener("click", approve);
  $("#permit-card").innerHTML = permit ? `<article class="permit"><span class="kicker">PERMIT NO. ${permit.sequence}</span><h3>舞台布景阻燃准用凭据</h3><p>批准人：${escapeHTML(permit.approvedBy)} · 签发于 ${new Date(permit.issuedAt).toLocaleString("zh-CN")}</p><p>准用布景：${permit.approvedUnitIds.map(unitName).map(escapeHTML).join("、")}</p><p>清单摘要</p><div class="digest">${escapeHTML(permit.manifestDigest)}</div><p>凭据摘要</p><div class="digest">${escapeHTML(permit.permitDigest)}</div><p>${detail.permitVerification?.valid ? "✓ 摘要链核验通过" : "⚠ 摘要链核验失败"}</p></article>` : "";
  $("#timeline").innerHTML = detail.timeline.map(event => `<div class="timeline-item"><strong>${escapeHTML(actionLabels[event.action] || event.action)}</strong><span>${escapeHTML(event.actor)} · 修订 ${event.revision} · ${new Date(event.occurredAt).toLocaleString("zh-CN")}</span></div>`).join("");
}
function setTab(name) { state.tab = name; $$(".tab-panel").forEach(panel => panel.classList.toggle("hidden", panel.id !== name)); $$(".steps button").forEach(button => button.classList.toggle("active", button.dataset.tab === name)); }
function openResult(unitId, checkCode) { const form = $("#result-form"); form.reset(); form.elements.unitId.value = unitId; form.elements.checkCode.value = checkCode; form.elements.inspector.value = "阻燃检查员"; $("#result-dialog").showModal(); }

function editUnit(id) {
  const unit=state.detail.aggregate.units.find(item=>item.id===id), form=$("#unit-form"); if(!unit)return;
  form.reset(); $("#unit-form-title").textContent="编辑布景单元"; form.elements.unitId.value=unit.id;
  ["unitCode","name","stageZone","materialClass","supplier","treatmentLot"].forEach(name=>form.elements[name].value=unit[name]);
  form.elements.evidenceList.value=unit.evidenceRefs.map(ref=>`${ref.name} | ${ref.digest}`).join("\n"); form.classList.remove("hidden"); form.scrollIntoView({behavior:"smooth"});
}
async function removeUnit(id) {
  if(!confirm("确认撤回这个草稿布景单元？"))return;
  await run(()=>request(`/api/v1/batches/${state.selected}/units/${encodeURIComponent(id)}`,{method:"DELETE",body:JSON.stringify({revision:state.detail.aggregate.batch.revision,actor:"制作协调员",idempotencyKey:key("remove-unit")})}),"布景登记已撤回");
}
async function preflightPlan() {
  try {
    const preview=await request(`/api/v1/batches/${state.selected}/plan/preflight`,{method:"POST",body:JSON.stringify({checkDefinitions:planDefinitions()})});
    state.planDigest=preview.confirmationDigest || ""; const summary=preview.summary;
    $("#preflight-result").className=`preflight mini ${preview.confirmable?"ok":"error"}`;
    $("#preflight-result").innerHTML=`<strong>${preview.confirmable?"覆盖预检通过":"存在阻断诊断"}</strong><p>布景 ${summary.unitCount} · 必检项 ${summary.requiredCheckCount} · 阻断项 ${summary.blockingCheckCount} · 总检查 ${summary.totalCheckCount}</p>${preview.diagnostics.map(item=>`<p>${escapeHTML(item.message)}</p>`).join("")}${preview.coverage.length?`<details><summary>查看 ${preview.coverage.length} 个覆盖单元格</summary>${preview.coverage.map(cell=>`<span class="pill pending">${escapeHTML(cell.unitCode)} × ${escapeHTML(cell.checkCode)}</span>`).join(" ")}</details>`:""}`;
    $("#submit-plan").disabled=!preview.confirmable;
  } catch(error) { state.planDigest=""; $("#submit-plan").disabled=true; toast(error.message,true); }
}

async function recordBatch() {
  const selected=$$('[data-select-cell]:checked'); if(!selected.length){toast("请先选择待检或可复测单元格",true);return;}
  const inspector=prompt("本轮检查人","阻燃检查员"); if(!inspector)return; const results=[];
  for(const cell of selected){ const label=`${unitName(cell.dataset.unit)} / ${cell.dataset.check}`; const outcome=prompt(`${label} 结论（pass 或 fail）`,"pass"); if(!outcome)return; const measuredValue=prompt(`${label} 测量值`,outcome==="pass"?"符合":"不符合"); if(measuredValue===null)return; const evidenceDigest=prompt(`${label} 证据摘要`); if(!evidenceDigest)return; results.push({unitId:cell.dataset.unit,checkCode:cell.dataset.check,outcome,measuredValue,evidenceDigest,inspector}); }
  await run(()=>request(`/api/v1/batches/${state.selected}/results/batch`,{method:"POST",body:JSON.stringify({revision:state.detail.aggregate.batch.revision,actor:inspector,idempotencyKey:key("result-batch"),results})}),`已原子保存 ${results.length} 项检查结果`);
}

async function openRemediation(resultId) {
  const owner = prompt("整改责任人", "布景制作负责人"); if (!owner) return;
  const due = new Date(Date.now() + 86400000).toISOString();
  await run(() => request(`/api/v1/batches/${state.selected}/remediations`, { method:"POST", body:JSON.stringify({ revision:state.detail.aggregate.batch.revision, actor:"阻燃检查员", idempotencyKey:key("open-rem"), remediation:{ checkResultId:resultId, owner, dueAt:due } }) }), "整改记录已创建");
}
async function completeRemediation(id) {
  const note = prompt("请输入整改说明"); if (!note) return; const digest = prompt("请输入新证据摘要"); if (!digest) return;
  await run(() => request(`/api/v1/batches/${state.selected}/remediations/${id}/complete`, { method:"POST", body:JSON.stringify({ revision:state.detail.aggregate.batch.revision, actor:"整改责任人", idempotencyKey:key("complete-rem"), actionNote:note, evidenceRefs:[{name:"整改证据",digest}] }) }), "整改已完成，可以发起复测");
}
async function changeRemediation(id) {
  const rem=state.detail.aggregate.remediations.find(item=>item.id===id); if(!rem)return;
  const owner=prompt("新责任人",rem.owner); if(!owner)return; const dueAt=prompt("新期限（ISO 8601，不得早于原期限）",new Date(new Date(rem.dueAt).getTime()+86400000).toISOString()); if(!dueAt)return; const reason=prompt("变更原因"); if(!reason)return;
  await run(()=>request(`/api/v1/batches/${state.selected}/remediations/${id}`,{method:"PATCH",body:JSON.stringify({revision:state.detail.aggregate.batch.revision,actor:"制作协调员",idempotencyKey:key("change-rem"),owner,dueAt:new Date(dueAt).toISOString(),reason})}),"整改责任与期限已调整");
}
async function approve() { const approvedBy = prompt("安全负责人姓名", "演出安全负责人"); if (!approvedBy) return; await run(() => request(`/api/v1/batches/${state.selected}/approve`, { method:"POST", body:JSON.stringify({ revision:state.detail.aggregate.batch.revision, actor:approvedBy, approvedBy, idempotencyKey:key("approve") }) }), "准用凭据已签发"); state.tab="permit"; }
function renderPermitLookup(data) {
  const v=data.verification, checks=v.manifest?.checks || [], units=v.manifest?.units || [], timeline=data.timeline || [];
  $("#permit-lookup-result").innerHTML=`<article class="permit"><h4>${escapeHTML(v.message)}</h4>${v.matched?`<p>凭据 #${v.permit.sequence} · 批次 ${escapeHTML(v.permit.batchId)}</p><p>清单摘要 ${v.manifestValid?"✓":"✗"} · 凭据摘要 ${v.permitValid?"✓":"✗"} · 前序链路 ${v.chainValid?"✓":"✗"}</p><p>冻结布景：${units.map(unit=>escapeHTML(unit.unitCode+" "+unit.name)).join("、")}</p><div class="table-wrap"><table><thead><tr><th>布景</th><th>检查</th><th>尝试</th><th>证据摘要</th><th>检查人</th></tr></thead><tbody>${checks.map(check=>`<tr><td>${escapeHTML(check.unitId)}</td><td>${escapeHTML(check.checkCode)}</td><td>${check.attempt}</td><td>${escapeHTML(check.evidenceDigest)}</td><td>${escapeHTML(check.inspector)}</td></tr>`).join("")}</tbody></table></div><h4>对应审计时间线</h4>${timeline.map(event=>`<p>${escapeHTML(actionLabels[event.action]||event.action)} · ${escapeHTML(event.actor)} · r${event.revision}</p>`).join("")}`:""}</article>`;
}
async function editBatch() {
  const batch=state.detail.aggregate.batch;
  const title=prompt("批次名称",batch.title); if(!title)return;
  const venue=prompt("场地",batch.venue); if(!venue)return;
  const coordinator=prompt("制作协调员",batch.coordinator); if(!coordinator)return;
  const performanceAt=prompt("计划上台时间（ISO 8601）",batch.performanceAt); if(!performanceAt)return;
  await run(()=>request(`/api/v1/batches/${state.selected}`,{method:"PATCH",body:JSON.stringify({revision:batch.revision,actor:"制作协调员",idempotencyKey:key("update-batch"),title,venue,coordinator,performanceAt:new Date(performanceAt).toISOString()})}),"批次信息已更新");
}
async function run(operation, success) { try { await operation(); toast(success); await refresh(); } catch (error) { toast(error.message, true); } }

$("#new-batch").addEventListener("click", () => $("#batch-dialog").showModal());
$$('[data-action="new-batch"]').forEach(node => node.addEventListener("click", () => $("#batch-dialog").showModal()));
$("#batch-form").addEventListener("submit", async event => { event.preventDefault(); const form = new FormData(event.target); const body = Object.fromEntries(form); body.performanceAt = new Date(body.performanceAt).toISOString(); body.idempotencyKey = key("create"); await run(() => request("/api/v1/batches", {method:"POST",body:JSON.stringify(body)}), "检查批次已创建"); $("#batch-dialog").close(); await loadBatches(true); });
$("#show-unit").addEventListener("click", () => { const form=$("#unit-form"); form.reset(); $("#unit-form-title").textContent="新增布景单元"; form.classList.remove("hidden"); });
$("#edit-batch").addEventListener("click", editBatch);
$('[data-action="cancel-unit"]').addEventListener("click", () => $("#unit-form").classList.add("hidden"));
$("#unit-form").addEventListener("submit", async event => { event.preventDefault(); const value=Object.fromEntries(new FormData(event.target)); const evidenceRefs=value.evidenceList.split("\n").filter(line=>line.trim()).map(line=>{const split=line.indexOf("|");return {name:(split<0?line:line.slice(0,split)).trim(),digest:(split<0?"":line.slice(split+1)).trim()};}); const unit={unitCode:value.unitCode,name:value.name,stageZone:value.stageZone,materialClass:value.materialClass,supplier:value.supplier,treatmentLot:value.treatmentLot,evidenceRefs}; const editing=Boolean(value.unitId),path=editing?`/api/v1/batches/${state.selected}/units/${encodeURIComponent(value.unitId)}`:`/api/v1/batches/${state.selected}/units`; await run(() => request(path,{method:editing?"PUT":"POST",body:JSON.stringify({revision:state.detail.aggregate.batch.revision,actor:"制作协调员",idempotencyKey:key(editing?"update-unit":"unit"),unit})}),editing?"布景登记已更新":"布景单元已登记"); state.planDigest=""; event.target.reset(); event.target.classList.add("hidden"); });
$("#add-definition").addEventListener("click", () => { $("#definition-list").append(definitionRow()); invalidatePreview(); });
$("#plan-form").addEventListener("input",invalidatePreview);
$("#preflight-plan").addEventListener("click",preflightPlan);
$("#plan-form").addEventListener("submit", async event => { event.preventDefault(); if(!state.planDigest){toast("请先完成覆盖预检",true);return;} const definitions=planDefinitions(event.target); const actor=event.target.elements.actor.value; await run(()=>request(`/api/v1/batches/${state.selected}/submit`,{method:"POST",body:JSON.stringify({revision:state.detail.aggregate.batch.revision,actor,idempotencyKey:key("submit"),confirmationDigest:state.planDigest,checkDefinitions:definitions})}),"方案已冻结并送检"); state.planDigest=""; state.tab="inspect"; });
$("#result-form").addEventListener("submit", async event => { event.preventDefault(); const value=Object.fromEntries(new FormData(event.target)); await run(()=>request(`/api/v1/batches/${state.selected}/results`,{method:"POST",body:JSON.stringify({revision:state.detail.aggregate.batch.revision,actor:value.inspector,idempotencyKey:key("result"),result:{unitId:value.unitId,checkCode:value.checkCode,outcome:value.outcome,measuredValue:value.measuredValue,evidenceDigest:value.evidenceDigest,inspector:value.inspector}})}),"检查结果已记录"); $("#result-dialog").close(); });
$$('.steps button').forEach(button => button.addEventListener("click", () => setTab(button.dataset.tab)));
$("#batch-record").addEventListener("click",recordBatch);
$("#progress-filter").addEventListener("submit",async event=>{event.preventDefault();state.filters=Object.fromEntries(new FormData(event.target));await refresh();});
$("#remediation-filter").addEventListener("submit",async event=>{event.preventDefault();state.remediationFilters=Object.fromEntries(new FormData(event.target));await refresh();});
$("#permit-lookup").addEventListener("submit",async event=>{event.preventDefault();try{const value=Object.fromEntries(new FormData(event.target)),query=new URLSearchParams();if(value.sequence)query.set("sequence",value.sequence);if(value.permitDigest)query.set("permitDigest",value.permitDigest.trim());renderPermitLookup(await request(`/api/v1/permits/lookup?${query}`));}catch(error){toast(error.message,true);}});
$("#verify-chain").addEventListener("click", async () => { try { const result=await request("/api/v1/permits/verify"); $("#chain-result").textContent=`${result.valid?"✓":"⚠"} ${result.message}（${result.count} 份）`; } catch(error) { toast(error.message,true); } });
loadBatches().catch(error => toast(error.message,true));
