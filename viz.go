package main

import (
	"bufio"
	"encoding/json"
	"regexp"
	"strings"
)

func parseDependencies(content string) GraphData {
	nodes := make(map[string]Node)
	var links []Link
	linkSet := make(map[string]bool)

	var currentItem string
	var currentType string
	itemRegex := regexp.MustCompile(`^(.+?)\s+\((func|type|method|var|const)\):$`)

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if matches := itemRegex.FindStringSubmatch(line); matches != nil {
			currentItem = matches[1]
			currentType = matches[2]
			nodes[currentItem] = Node{
				ID:    currentItem,
				Type:  currentType,
				Group: classifyNode(currentItem, currentType),
			}

			if currentType == kindMethod && strings.Contains(currentItem, ".") {
				parts := strings.Split(currentItem, ".")
				if len(parts) == 2 {
					typeName := parts[0]
					if _, exists := nodes[typeName]; !exists {
						nodes[typeName] = Node{ID: typeName, Type: kindType, Group: classifyNode(typeName, kindType)}
					}
					linkKey := currentItem + "->" + typeName
					if !linkSet[linkKey] {
						links = append(links, Link{Source: currentItem, Target: typeName})
						linkSet[linkKey] = true
					}
				}
			}
			continue
		}

		if strings.HasPrefix(line, "- ") && currentItem != "" {
			target := strings.TrimSpace(line[2:])
			if _, exists := nodes[target]; !exists {
				nodes[target] = Node{ID: target, Type: kindUnknown, Group: classifyNode(target, kindUnknown)}
			}
			linkKey := currentItem + "->" + target
			if !linkSet[linkKey] {
				links = append(links, Link{Source: currentItem, Target: target})
				linkSet[linkKey] = true
			}
		}
	}

	nodeSlice := make([]Node, 0, len(nodes))
	for _, node := range nodes {
		nodeSlice = append(nodeSlice, node)
	}
	return GraphData{Nodes: nodeSlice, Links: links}
}

func classifyNode(name string, typ string) string {
	if strings.HasPrefix(name, "Test") {
		return "test"
	}
	if strings.Contains(name, ".") {
		return "method"
	}
	return typ
}

// generateHTML is the same as in your viz.go file
func generateHTML(data GraphData) string {
	dataJSON, _ := json.Marshal(data)
	htmlTemplate := `<!DOCTYPE html>
<html>
<head>
   <meta charset="utf-8">
   <title>Go Package Dependency Graph</title>
   <script src="https://d3js.org/d3.v7.min.js"></script>
   <style>
       body { margin: 0; padding: 20px; font-family: Arial, sans-serif; background: #1a1a1a; color: #fff; overflow: hidden; }
       #graph { border: 1px solid #444; background: #222; width: 100%; height: calc(100vh - 150px); }
       .controls { margin-bottom: 20px; padding: 15px; background: #2a2a2a; border-radius: 5px; display: flex; align-items: center; flex-wrap: wrap; gap: 15px; }
       .node { cursor: pointer; stroke: #fff; stroke-width: 1.5px; transition: opacity 0.2s; }
       .node.selected { stroke: #fff; stroke-width: 4px; filter: drop-shadow(0 0 5px #fff); }
       .link { stroke: #999; stroke-opacity: 0.4; fill: none; pointer-events: none; }
       .node-label { font-size: 10px; pointer-events: none; fill: #fff; }
       .legend { position: fixed; right: 20px; top: 160px; background: #2a2a2a; padding: 15px; border-radius: 5px; border: 1px solid #444; }
       .legend-item { margin: 5px 0; display: flex; align-items: center; font-size: 12px; cursor: pointer; transition: opacity 0.2s; }
       .legend-item.inactive { opacity: 0.3; }
       .legend-color { width: 12px; height: 12px; margin-right: 10px; border-radius: 50%; }
       .info { position: fixed; left: 20px; top: 160px; background: #2a2a2a; padding: 15px; border-radius: 5px; border: 1px solid #444; width: 280px; max-height: 70vh; display: flex; flex-direction: column; }
       .sidebar-scroll { flex-grow: 1; overflow-y: auto; padding-right: 5px; }
       .sidebar-list { list-style: none; padding: 0; margin: 10px 0; }
       .sidebar-list li { padding: 4px 8px; margin: 2px 0; background: #333; border-radius: 3px; font-size: 11px; cursor: pointer; border-left: 3px solid transparent; }
       .sidebar-list li:hover { background: #444; }
       .li-outgoing { border-left-color: #4ecdc4 !important; }
       .li-incoming { border-left-color: #ff6b6b !important; }
       .li-search { border-left-color: #fff !important; }
       .li-selected { border-left-color: #ffd700 !important; background: #3d3d29 !important; }
       .muted { opacity: 0.1 !important; }
       .highlight-out { stroke: #4ecdc4 !important; stroke-opacity: 1 !important; stroke-width: 3px !important; }
       .highlight-in { stroke: #ff6b6b !important; stroke-opacity: 1 !important; stroke-width: 3px !important; }
       .highlight-internal { stroke: #ffffff !important; stroke-opacity: 1 !important; stroke-width: 3px !important; }
       button { background: #444; color: white; border: 1px solid #666; padding: 5px 10px; cursor: pointer; border-radius: 3px; }
       button:hover { background: #555; }
       #search-box { width: 100%; padding: 8px; background: #333; border: 1px solid #555; color: #fff; border-radius: 3px; margin-bottom: 10px; box-sizing: border-box; }
       #search-results { max-height: 240px; overflow-y: auto; border-bottom: 1px solid #444; margin-bottom: 10px; flex-shrink: 0; }
       #node-info { flex-shrink: 3; }
       .section-header { font-size: 12px; color: #aaa; text-transform: uppercase; margin-top: 15px; display: block; border-bottom: 1px solid #444; }
   </style>
</head>
<body>
   <div class="controls">
       <h3 style="margin:0">Go Dependencies</h3>
       <label><input type="checkbox" id="show-labels" checked> Labels</label>
       <label>Dist: <input type="range" id="link-distance" min="30" max="300" value="120"></label>
       <label>Charge: <input type="range" id="charge" min="-800" max="-50" value="-300"></label>
       <button id="reset-focus">Reset Focus</button>
   </div>
   
   <div class="legend">
       <div class="legend-item" data-group="test"><div class="legend-color" style="background: #ff6b6b;"></div>Tests</div>
       <div class="legend-item" data-group="type"><div class="legend-color" style="background: #4ecdc4;"></div>Types</div>
       <div class="legend-item" data-group="method"><div class="legend-color" style="background: #45b7d1;"></div>Methods</div>
       <div class="legend-item" data-group="func"><div class="legend-color" style="background: #96ceb4;"></div>Functions</div>
       <div class="legend-item" data-group="const"><div class="legend-color" style="background: #ac4ace;"></div>Constants</div>
       <div class="legend-item" data-group="var"><div class="legend-color" style="background: #39b20d;"></div>Variables</div>
       <hr style="border: 0; border-top: 1px solid #444;">
       <div class="legend-item" style="cursor:default"><div style="width:15px; height:2px; background:#4ecdc4; margin-right:5px;"></div>External Out</div>
       <div class="legend-item" style="cursor:default"><div style="width:15px; height:2px; background:#ff6b6b; margin-right:5px;"></div>External In</div>
       <div class="legend-item" style="cursor:default"><div style="width:15px; height:2px; background:#ffffff; margin-right:5px;"></div>Internal Link</div>
   </div>
   
   <div class="info">
       <input type="text" id="search-box" placeholder="Search nodes...">
       <div id="search-results"></div>
       <div class="sidebar-scroll" id="node-info"><i>Click a node to see details</i></div>
   </div>
   
   <svg id="graph"></svg>

   <script>
       const data = DATA_PLACEHOLDER;
       const width = window.innerWidth;
       const height = window.innerHeight;
       let selectedNodeIds = new Set();
       let activeGroups = new Set(['test', 'type', 'method', 'func', 'var', 'const']);

       const svg = d3.select("#graph").on("click", (e) => { if(e.target.tagName === 'svg') resetFocus(); });
       const g = svg.append("g");
       const defs = svg.append("defs");
       
       const createMarker = (id, color) => {
           defs.append("marker").attr("id", id).attr("viewBox", "0 -5 10 10").attr("refX", 5).attr("refY", 0)
               .attr("markerWidth", 6).attr("markerHeight", 6).attr("orient", "auto")
               .append("path").attr("d", "M0,-5L10,0L0,5").attr("fill", color);
       };
       createMarker("arrow-default", "#999");
       createMarker("arrow-outgoing", "#4ecdc4");
       createMarker("arrow-incoming", "#ff6b6b");
       createMarker("arrow-internal", "#ffffff");

       const zoom = d3.zoom().scaleExtent([0.1, 4]).on("zoom", (e) => g.attr("transform", e.transform));
       svg.call(zoom);

       const colorMap = { 'test': '#ff6b6b', 'type': '#4ecdc4', 'method': '#45b7d1', 'func': '#96ceb4', 'const': '#ac4ace', 'var': '#39b20d' };
       let simulation, link, node, label, midArrow;

       // --- URL Persistence Logic ---
       function updateURL() {
           const params = new URLSearchParams();
           if (selectedNodeIds.size > 0) params.set('sel', Array.from(selectedNodeIds).join(','));
           params.set('groups', Array.from(activeGroups).join(','));
           params.set('labels', document.getElementById("show-labels").checked);
           params.set('dist', document.getElementById("link-distance").value);
           params.set('charge', document.getElementById("charge").value);
           window.history.pushState(null, '', '#' + params.toString());
       }

       function loadFromURL() {
           const hash = window.location.hash.substring(1);
           if (!hash) return;
           const params = new URLSearchParams(hash);
           
           if (params.has('sel')) selectedNodeIds = new Set(params.get('sel').split(','));
           if (params.has('groups')) {
               activeGroups = new Set(params.get('groups').split(','));
               document.querySelectorAll(".legend-item[data-group]").forEach(item => {
                   const group = item.getAttribute("data-group");
                   item.classList.toggle("inactive", !activeGroups.has(group));
               });
           }
           if (params.has('labels')) document.getElementById("show-labels").checked = params.get('labels') === 'true';
           if (params.has('dist')) document.getElementById("link-distance").value = params.get('dist');
           if (params.has('charge')) document.getElementById("charge").value = params.get('charge');
       }

       function updateGraph() {
           const filteredNodes = data.nodes.filter(n => activeGroups.has(n.group));
           const nodeIds = new Set(filteredNodes.map(n => n.id));
           const filteredLinks = data.links.filter(l => nodeIds.has(l.source.id || l.source) && nodeIds.has(l.target.id || l.target));

           g.selectAll("*").remove();

           link = g.append("g").selectAll("line").data(filteredLinks).join("line").attr("class", "link");
           midArrow = g.append("g").selectAll("path").data(filteredLinks).join("path").attr("class", "mid-arrow").attr("marker-end", "url(#arrow-default)");

           node = g.append("g").selectAll("circle").data(filteredNodes).join("circle")
               .attr("class", "node").attr("r", d => d.group === 'test' ? 6 : 10)
               .attr("fill", d => colorMap[d.group] || '#999')
               .call(d3.drag().on("start", dragstarted).on("drag", dragged).on("end", dragended))
               .on("click", (e, d) => { e.stopPropagation(); handleSelectionLogic(d.id, e.shiftKey); });

           label = g.append("g").selectAll("text").data(filteredNodes).join("text")
               .attr("class", "node-label").attr("dx", 14).attr("dy", 4).text(d => d.id)
               .style("display", document.getElementById("show-labels").checked ? "block" : "none");

           simulation = d3.forceSimulation(filteredNodes)
               .force("link", d3.forceLink(filteredLinks).id(d => d.id).distance(+document.getElementById("link-distance").value))
               .force("charge", d3.forceManyBody().strength(+document.getElementById("charge").value))
               .force("center", d3.forceCenter(width / 2, height / 2)).force("collision", d3.forceCollide().radius(25));

           simulation.on("tick", () => {
               link.attr("x1", d => d.source.x).attr("y1", d => d.source.y).attr("x2", d => d.target.x).attr("y2", d => d.target.y);
               midArrow.attr("d", d => {
                   const midX = (d.source.x + d.target.x) / 2, midY = (d.source.y + d.target.y) / 2;
                   const angle = Math.atan2(d.target.y - d.source.y, d.target.x - d.source.x);
                   return ` + "`" + `M${midX},${midY} L${midX + Math.cos(angle)},${midY + Math.sin(angle)}` + "`" + `;
               });
               node.attr("cx", d => d.x).attr("cy", d => d.y);
               label.attr("x", d => d.x).attr("y", d => d.y);
           });
           if(selectedNodeIds.size > 0) applyFocus();
       }

       function handleSelectionLogic(id, isShift) {
           const newSelections = new Set();
           const targetNode = data.nodes.find(n => n.id === id);
       
           // Feature: Include all methods of a type in the selection
           if (targetNode && targetNode.type === 'type') {
               newSelections.add(id);
               data.nodes.forEach(n => {
                   if (n.type === 'method' && n.id.startsWith(id + ".")) {
                       newSelections.add(n.id);
                   }
               });
           } else {
               newSelections.add(id);
           }
       
           if (isShift) {
               newSelections.forEach(sid => {
                   if (selectedNodeIds.has(sid)) selectedNodeIds.delete(sid);
                   else selectedNodeIds.add(sid);
               });
           } else {
               selectedNodeIds.clear();
               newSelections.forEach(sid => selectedNodeIds.add(sid));
           }
       
           if (selectedNodeIds.size === 0) resetFocus(); else applyFocus();
           updateURL();
       }

       function applyFocus() {
           const connectedNodes = new Set(selectedNodeIds);
           const extOut = data.links.filter(l => selectedNodeIds.has(l.source.id || l.source) && !selectedNodeIds.has(l.target.id || l.target));
           const extIn = data.links.filter(l => !selectedNodeIds.has(l.source.id || l.source) && selectedNodeIds.has(l.target.id || l.target));

           extOut.forEach(l => connectedNodes.add(l.target.id || l.target));
           extIn.forEach(l => connectedNodes.add(l.source.id || l.source));

           node.classed("muted", d => !connectedNodes.has(d.id)).classed("selected", d => selectedNodeIds.has(d.id));
           label.classed("muted", d => !connectedNodes.has(d.id));
           
           link.classed("muted", true).classed("highlight-out", false).classed("highlight-in", false).classed("highlight-internal", false);
           midArrow.classed("muted", true).attr("marker-end", "url(#arrow-default)");

           link.filter(l => selectedNodeIds.has(l.source.id || l.source) && !selectedNodeIds.has(l.target.id || l.target)).classed("muted", false).classed("highlight-out", true);
           midArrow.filter(l => selectedNodeIds.has(l.source.id || l.source) && !selectedNodeIds.has(l.target.id || l.target)).classed("muted", false).attr("marker-end", "url(#arrow-outgoing)");
           link.filter(l => !selectedNodeIds.has(l.source.id || l.source) && selectedNodeIds.has(l.target.id || l.target)).classed("muted", false).classed("highlight-in", true);
           midArrow.filter(l => !selectedNodeIds.has(l.source.id || l.source) && selectedNodeIds.has(l.target.id || l.target)).classed("muted", false).attr("marker-end", "url(#arrow-incoming)");
           link.filter(l => selectedNodeIds.has(l.source.id || l.source) && selectedNodeIds.has(l.target.id || l.target)).classed("muted", false).classed("highlight-internal", true);
           midArrow.filter(l => selectedNodeIds.has(l.source.id || l.source) && selectedNodeIds.has(l.target.id || l.target)).classed("muted", false).attr("marker-end", "url(#arrow-internal)");

           updateSidebar(extIn, extOut);
       }

       function resetFocus() {
           selectedNodeIds.clear();
           node.classed("muted", false).classed("selected", false);
           label.classed("muted", false);
           link.classed("muted", false).classed("highlight-out", false).classed("highlight-in", false).classed("highlight-internal", false);
           midArrow.classed("muted", false).attr("marker-end", "url(#arrow-default)");
           document.getElementById("node-info").innerHTML = "<i>Click a node to see details</i>";
           document.getElementById("search-box").value = "";
           document.getElementById("search-results").innerHTML = "";
           updateURL();
       }

       function updateSidebar(incoming, outgoing) {
           const info = document.getElementById("node-info");
           let html = "<h3>Focus Mode</h3>";
           
           // Deduplicate and Sort IDs
	        const getSortedIds = (links, key) => {
	            return Array.from(new Set(links.map(l => (typeof l[key] === 'object' ? l[key].id : l[key])))).sort();
	        };
	    
	        const outIds = getSortedIds(outgoing, 'target');
	        const inIds = getSortedIds(incoming, 'source');
	        const selIds = Array.from(selectedNodeIds).sort();
           
           html += "<span class='section-header'>Selected</span><ul class='sidebar-list'>";
           selIds.forEach(id => html += ` + "`" + `<li class='li-selected' onclick="handleSelectionLogic('${id}', event.shiftKey)">${id}</li>` + "`" + `);
           html += ` + "`" + `</ul><span class='section-header'>Outgoing (${outIds.length})</span><ul class='sidebar-list'>` + "`" + `;
           outIds.forEach(id => html += ` + "`" + `<li class='li-outgoing' onclick="handleSelectionLogic('${id}', event.shiftKey)">${id}</li>` + "`" + `);
           html += ` + "`" + `</ul><span class='section-header'>Incoming (${inIds.length})</span><ul class='sidebar-list'>` + "`" + `;
           inIds.forEach(id => html += ` + "`" + `<li class='li-incoming' onclick="handleSelectionLogic('${id}', event.shiftKey)">${id}</li>` + "`" + `);
           info.innerHTML = html + "</ul>";
       }

       document.querySelectorAll(".legend-item[data-group]").forEach(item => {
           item.addEventListener("click", () => {
               const group = item.getAttribute("data-group");
               if (activeGroups.has(group)) { activeGroups.delete(group); item.classList.add("inactive"); }
               else { activeGroups.add(group); item.classList.remove("inactive"); }
               updateGraph();
               updateURL();
           });
       });

       document.getElementById("search-box").addEventListener("input", (e) => {
           const term = e.target.value.toLowerCase();
           const resultsDiv = document.getElementById("search-results");
           if (term.length < 1) { resultsDiv.innerHTML = ""; return; }
           const matches = data.nodes.filter(n => n.id.toLowerCase().includes(term));
           let html = "<ul class='sidebar-list'>";
           matches.forEach(m => html += ` + "`" + `<li class='li-search' onclick="handleSelectionLogic('${m.id}', event.shiftKey)">${m.id}</li>` + "`" + `);
           resultsDiv.innerHTML = html + "</ul>";
       });

       function dragstarted(event) { if (!event.active) simulation.alphaTarget(0.3).restart(); event.subject.fx = event.subject.x; event.subject.fy = event.subject.y; }
       function dragged(event) { event.subject.fx = event.x; event.subject.fy = event.y; }
       function dragended(event) { if (!event.active) simulation.alphaTarget(0); event.subject.fx = null; event.subject.fy = null; }

       document.getElementById("show-labels").addEventListener("change", (e) => { label.style("display", e.target.checked ? "block" : "none"); updateURL(); });
       document.getElementById("link-distance").addEventListener("input", (e) => { simulation.force("link").distance(+e.target.value); simulation.alpha(0.3).restart(); updateURL(); });
       document.getElementById("charge").addEventListener("input", (e) => { simulation.force("charge").strength(+e.target.value); simulation.alpha(0.3).restart(); updateURL(); });
       document.getElementById("reset-focus").addEventListener("click", resetFocus);

       // Initial Load
       loadFromURL();
       updateGraph();
       window.handleSelectionLogic = handleSelectionLogic;
       window.onhashchange = () => { loadFromURL(); updateGraph(); };
   </script>
</body>
</html>`
	return strings.Replace(htmlTemplate, "DATA_PLACEHOLDER", string(dataJSON), 1)
}
