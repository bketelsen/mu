package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"mu/apps"
	"mu/internal/app"
	"mu/internal/auth"
	"mu/internal/data"
	"mu/internal/flag"
)

// ConsoleHandler provides an admin console.
func ConsoleHandler(w http.ResponseWriter, r *http.Request) {
	_, _, err := auth.RequireAdmin(r)
	if err != nil {
		app.Forbidden(w, r, "Admin access required")
		return
	}

	// POST: run command and return result
	if r.Method == "POST" {
		var cmd string
		if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			var req struct {
				Cmd string `json:"cmd"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			cmd = strings.TrimSpace(req.Cmd)
		} else {
			r.ParseForm()
			cmd = strings.TrimSpace(r.FormValue("cmd"))
		}
		output := ""
		if cmd != "" {
			output = runCommand(cmd)
		}
		if app.WantsJSON(r) || strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			app.RespondJSON(w, map[string]string{"output": output})
			return
		}
		http.Redirect(w, r, "/admin/console?cmd="+url.QueryEscape(cmd)+"&output="+url.QueryEscape(output), http.StatusSeeOther)
		return
	}

	// GET: render page
	prevCmd := r.URL.Query().Get("cmd")
	prevOutput := r.URL.Query().Get("output")

	var sb strings.Builder
	sb.WriteString(`<div class="card" style="background:#1a1a1a;color:#e0e0e0;font-family:'SF Mono','Fira Code',monospace;padding:16px;border:none">`)

	// Output area
	sb.WriteString(`<div id="cout" style="font-size:13px;white-space:pre-wrap;max-height:60vh;overflow-y:auto;margin-bottom:12px">`)
	if prevOutput != "" {
		sb.WriteString(fmt.Sprintf(`<span style="color:#888">&gt; %s</span>
%s`, esc(prevCmd), esc(prevOutput)))
	}
	sb.WriteString(`</div>`)

	// Input — form for fallback, JS for interactive
	sb.WriteString(`<form method="POST" action="/admin/console" id="cf" style="display:flex;gap:8px">`)
	sb.WriteString(`<span style="color:#888;line-height:32px">&gt;</span>`)
	sb.WriteString(`<input type="text" name="cmd" id="ci" autocomplete="off" autofocus style="flex:1;background:transparent;border:none;color:#e0e0e0;font-family:inherit;font-size:13px;outline:none;padding:6px 0">`)
	sb.WriteString(`<button type="submit" id="cb" style="background:#333;color:#e0e0e0;border:none;border-radius:4px;padding:4px 12px;font-family:inherit;font-size:12px;cursor:pointer">run</button>`)
	sb.WriteString(`</form>`)

	sb.WriteString(`<div style="margin-top:8px;font-size:11px;color:#555">help · apps · search · stats</div>`)
	sb.WriteString(`</div>`)

	// JS: intercept form, use fetch, append output inline
	sb.WriteString(`<script>
(function(){
  var form=document.getElementById('cf');
  var input=document.getElementById('ci');
  var out=document.getElementById('cout');
  var hist=[];
  var hi=-1;

  function run(){
    var cmd=input.value.trim();
    if(!cmd)return;
    hist.unshift(cmd);
    hi=-1;
    out.innerHTML+='<span style="color:#888">&gt; '+esc(cmd)+'</span>\n';
    input.value='';
    fetch('/admin/console',{method:'POST',body:JSON.stringify({cmd:cmd}),headers:{'Content-Type':'application/json'}})
    .then(function(r){return r.json()})
    .then(function(j){
      out.innerHTML+=esc(j.output)+'\n';
      out.scrollTop=out.scrollHeight;
    })
    .catch(function(e){
      out.innerHTML+='<span style="color:#c00">Error: '+esc(e.message)+'</span>\n';
    });
  }

  form.addEventListener('submit',function(e){
    e.preventDefault();
    run();
  });

  input.addEventListener('keydown',function(e){
    if(e.key==='ArrowUp'&&hist.length>0){
      hi=Math.min(hi+1,hist.length-1);
      input.value=hist[hi];
      e.preventDefault();
    }else if(e.key==='ArrowDown'){
      hi=Math.max(hi-1,-1);
      input.value=hi>=0?hist[hi]:'';
      e.preventDefault();
    }
  });

  function esc(s){
    var d=document.createElement('div');
    d.textContent=s;
    return d.innerHTML;
  }
})();
</script>`)

	html := app.RenderHTMLForRequest("Console", "Admin Console", sb.String(), r)
	w.Write([]byte(html))
}

func runCommand(cmd string) string {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return ""
	}

	arg := func(i int) string {
		if i < len(parts) {
			return parts[i]
		}
		return ""
	}
	rest := func(i int) string {
		if i < len(parts) {
			return strings.Join(parts[i:], " ")
		}
		return ""
	}

	switch parts[0] {

	// --- Apps ---
	case "apps":
		allApps := apps.GetPublicApps()
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%d public apps\n", len(allApps)))
		for _, a := range allApps {
			sb.WriteString(fmt.Sprintf("  %s — %s (by %s, %d launches)\n", a.Slug, a.Name, a.Author, a.Installs))
		}
		return sb.String()

	case "app":
		if arg(1) == "" {
			return "usage: app <slug>"
		}
		a := apps.GetApp(arg(1))
		if a == nil {
			return "App not found"
		}
		return fmt.Sprintf("Slug: %s\nName: %s\nAuthor: %s (%s)\nPublic: %v\nInstalls: %d\nCreated: %s\nHTML: %d bytes",
			a.Slug, a.Name, a.Author, a.AuthorID, a.Public, a.Installs, a.CreatedAt.Format("2 Jan 2006"), len(a.HTML))

	// --- Content ---
	case "search":
		if arg(1) == "" {
			return "usage: search <query>"
		}
		results := data.Search(rest(1), 20)
		if len(results) == 0 {
			return "No results."
		}
		var sb strings.Builder
		for _, r := range results {
			sb.WriteString(fmt.Sprintf("[%s] %s — %s\n", r.Type, r.ID, r.Title))
		}
		return sb.String()

	case "delete":
		if arg(1) == "" || arg(2) == "" {
			return "usage: delete <type> <id>"
		}
		if err := data.Delete(arg(1), rest(2)); err != nil {
			return "Error: " + err.Error()
		}
		return fmt.Sprintf("Deleted %s %s", arg(1), rest(2))

	case "flags":
		flagged := flag.GetAll()
		if len(flagged) == 0 {
			return "No flagged content."
		}
		var sb strings.Builder
		for _, f := range flagged {
			sb.WriteString(fmt.Sprintf("[%s] %s — %d flags, hidden: %v\n", f.ContentType, f.ContentID, f.FlagCount, f.Flagged))
		}
		return sb.String()

	// --- System ---
	case "stats":
		stats := data.GetStats()
		allApps := apps.GetPublicApps()
		return fmt.Sprintf("Apps: %d\nIndex: %d entries\nSQLite: %v",
			len(allApps), stats.TotalEntries, stats.UsingSQLite)

	case "types":
		return strings.Join(data.DeleteTypes(), ", ")

	case "help":
		return `Apps:     apps · app <slug>
Content:  search <query> · delete <type> <id> · flags
System:   stats · types · help`

	default:
		return fmt.Sprintf("Unknown: %s. Type help.", parts[0])
	}
}

func esc(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}
