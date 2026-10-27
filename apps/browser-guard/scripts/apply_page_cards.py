import os

new_cards = '''\t\t\t\t\t\t{/* POLICY 1: File Upload Policy (Includes Block All Files & File Type / Extension Filtering) */}
\t\t\t\t\t\t<Card className="border border-border bg-card overflow-hidden">
\t\t\t\t\t\t\t<div
\t\t\t\t\t\t\t\tclassName="flex items-center justify-between p-4 cursor-pointer select-none hover:bg-muted/30 transition-colors"
\t\t\t\t\t\t\t\tonClick={() => togglePolicyExpanded("upload")}
\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t<div className="flex items-center gap-3.5 min-w-0">
\t\t\t\t\t\t\t\t\t<div className="p-2.5 rounded-lg border border-border bg-muted/40 text-muted-foreground">
\t\t\t\t\t\t\t\t\t\t<Upload className="h-5 w-5" />
\t\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t\t\t<div>
\t\t\t\t\t\t\t\t\t\t<div className="flex items-center gap-2">
\t\t\t\t\t\t\t\t\t\t\t<p className="text-sm font-semibold">1. File Upload Policy</p>
\t\t\t\t\t\t\t\t\t\t\t{controls.block_upload ? (
\t\t\t\t\t\t\t\t\t\t\t\t<Badge variant="outline" className="text-[10px] px-2 py-0.5 border-emerald-500/50 text-emerald-600 dark:text-emerald-400 bg-emerald-500/10">Block All Files</Badge>
\t\t\t\t\t\t\t\t\t\t\t) : controls.block_file_extension ? (
\t\t\t\t\t\t\t\t\t\t\t\t<Badge variant="outline" className="text-[10px] px-2 py-0.5 border-blue-500/50 text-blue-600 dark:text-blue-400 bg-blue-500/10">
\t\t\t\t\t\t\t\t\t\t\t\t\t{controls.file_extension_mode === "allow_listed" ? "Allowlist Active" : "Blocklist Active"}
\t\t\t\t\t\t\t\t\t\t\t\t</Badge>
\t\t\t\t\t\t\t\t\t\t\t) : (
\t\t\t\t\t\t\t\t\t\t\t\t<Badge variant="outline" className="text-[10px] text-muted-foreground">Disabled</Badge>
\t\t\t\t\t\t\t\t\t\t\t)}
\t\t\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t\t\t\t<p className="text-xs text-muted-foreground mt-0.5">
\t\t\t\t\t\t\t\t\t\t\tBlock all file attachments or filter specific file extensions on monitored AI platforms.
\t\t\t\t\t\t\t\t\t\t</p>
\t\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t\t<div className="flex items-center gap-3 shrink-0" onClick={(e) => e.stopPropagation()}>
\t\t\t\t\t\t\t\t\t<Switch
\t\t\t\t\t\t\t\t\t\tchecked={!!(controls.block_upload || controls.block_file_extension)}
\t\t\t\t\t\t\t\t\t\tonCheckedChange={(val) => {
\t\t\t\t\t\t\t\t\t\t\tif (val) {
\t\t\t\t\t\t\t\t\t\t\t\tsetExpandedPolicies((p) => ({ ...p, upload: true }));
\t\t\t\t\t\t\t\t\t\t\t\tvoid patchControlWithErrorToast({ block_upload: true, block_file_extension: false }, "Could not enable file upload blocking");
\t\t\t\t\t\t\t\t\t\t\t} else {
\t\t\t\t\t\t\t\t\t\t\t\tvoid patchControlWithErrorToast({ block_upload: false, block_file_extension: false }, "Could not disable file upload blocking");
\t\t\t\t\t\t\t\t\t\t\t}
\t\t\t\t\t\t\t\t\t\t}}
\t\t\t\t\t\t\t\t\t\taria-label="Toggle file upload blocking"
\t\t\t\t\t\t\t\t\t/>
\t\t\t\t\t\t\t\t\t<button
\t\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\t\tonClick={() => togglePolicyExpanded("upload")}
\t\t\t\t\t\t\t\t\t\tclassName="p-1 rounded-md text-muted-foreground hover:text-foreground transition-transform"
\t\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t\t<ChevronDown className={`h-4 w-4 transition-transform duration-200 ${expandedPolicies.upload ? "rotate-180" : ""}`} />
\t\t\t\t\t\t\t\t\t</button>
\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t</div>

\t\t\t\t\t\t\t{expandedPolicies.upload && (
\t\t\t\t\t\t\t\t<div className="p-4 pt-0 border-t border-border/60 space-y-4 mt-2">
\t\t\t\t\t\t\t\t\t{/* Upload Mode Selector */}
\t\t\t\t\t\t\t\t\t<div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pt-3">
\t\t\t\t\t\t\t\t\t\t<div className="space-y-0.5">
\t\t\t\t\t\t\t\t\t\t\t<Label className="text-xs font-semibold">Upload Restriction Mode</Label>
\t\t\t\t\t\t\t\t\t\t\t<p className="text-[11px] text-muted-foreground">
\t\t\t\t\t\t\t\t\t\t\t\tChoose whether to block all file uploads entirely or filter by specific file extensions.
\t\t\t\t\t\t\t\t\t\t\t</p>
\t\t\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t\t\t\t<div className="flex items-center gap-1.5 bg-muted/30 border border-border p-1 rounded-lg text-xs shrink-0">
\t\t\t\t\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\t\t\t\t\tvariant={controls.block_upload ? "default" : "ghost"}
\t\t\t\t\t\t\t\t\t\t\t\tclassName="h-6 text-[11px] px-2.5 py-0"
\t\t\t\t\t\t\t\t\t\t\t\tonClick={() => void patchControlWithErrorToast({ block_upload: true, block_file_extension: false }, "Could not switch to Block All Files")}
\t\t\t\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t\t\t\tBlock All Files
\t\t\t\t\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\t\t\t\t\tvariant={controls.block_file_extension ? "default" : "ghost"}
\t\t\t\t\t\t\t\t\t\t\t\tclassName="h-6 text-[11px] px-2.5 py-0"
\t\t\t\t\t\t\t\t\t\t\t\tonClick={() => void patchControlWithErrorToast({ block_file_extension: true, block_upload: false }, "Could not switch to Filter by File Type")}
\t\t\t\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t\t\t\tFilter by File Type
\t\t\t\t\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t\t\t</div>

\t\t\t\t\t\t\t\t\t{/* MODE 1: Block All Files */}
\t\t\t\t\t\t\t\t\t{controls.block_upload && (
\t\t\t\t\t\t\t\t\t\t<div className="rounded-lg border border-border/70 bg-muted/15 p-4 space-y-3.5">
\t\t\t\t\t\t\t\t\t\t\t<div className="space-y-1.5">
\t\t\t\t\t\t\t\t\t\t\t\t<div className="flex items-center justify-between">
\t\t\t\t\t\t\t\t\t\t\t\t\t<Label className="text-xs font-semibold">Upload Warning Message</Label>
\t\t\t\t\t\t\t\t\t\t\t\t\t{!uploadWarningEditing && (controls.upload_warning || "").trim() ? (
\t\t\t\t\t\t\t\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\tvariant="ghost"
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\tclassName="h-6 text-xs text-muted-foreground hover:text-foreground"
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\tonClick={() => {
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\tsetUploadWarningError("");
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\tsetUploadWarningDraft(controls.upload_warning || "");
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t\tsetUploadWarningEditing(true);
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t}}
\t\t\t\t\t\t\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\t<Pencil className="h-3 w-3 mr-1" />
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\tEdit
\t\t\t\t\t\t\t\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t\t\t\t\t\t\t\t) : null}
\t\t\t\t\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t\t\t\t\t\t{uploadWarningEditing || !(controls.upload_warning || "").trim() ? (
\t\t\t\t\t\t\t\t\t\t\t\t\t<div className="space-y-2">
\t\t\t\t\t\t\t\t\t\t\t\t\t\t<Input
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\tplaceholder="e.g. File uploads are blocked by corporate policy."
\t\t\t\t\t\t\t\t\t\t\tvalue={uploadWarningDraft}
\t\t\t\t\t\t\t\t\t\t\tonChange={(e) => setUploadWarningDraft(e.target.value)}
\t\t\t\t\t\t\t\t\t\t\tclassName="text-xs font-sans"
\t\t\t\t\t\t\t\t\t\t\tonKeyDown={(e) => {
\t\t\t\t\t\t\t\t\t\t\t\tif (e.key === "Enter") {
\t\t\t\t\t\t\t\t\t\t\t\t\te.preventDefault();
\t\t\t\t\t\t\t\t\t\t\t\t\tvoid saveUploadWarning();
\t\t\t\t\t\t\t\t\t\t\t\t}
\t\t\t\t\t\t\t\t\t\t\t}}
\t\t\t\t\t\t\t\t\t\t/>
\t\t\t\t\t\t\t\t\t\t<div className="flex items-center justify-between gap-2">
\t\t\t\t\t\t\t\t\t\t\t<p className="text-[11px] text-muted-foreground">
\t\t\t\t\t\t\t\t\t\t\t\tMessage shown when file upload is blocked.
\t\t\t\t\t\t\t\t\t\t\t</p>
\t\t\t\t\t\t\t\t\t\t\t<div className="flex items-center gap-2">
\t\t\t\t\t\t\t\t\t\t\t\t{uploadWarningEditing && (controls.upload_warning || "").trim() ? (
\t\t\t\t\t\t\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\t\t\t\t\t\t\tvariant="ghost"
\t\t\t\t\t\t\t\t\t\t\t\t\t\tclassName="h-7 text-xs"
\t\t\t\t\t\t\t\t\t\t\t\t\t\tonClick={() => {
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\tsetUploadWarningError("");
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\tsetUploadWarningDraft(controls.upload_warning || "");
\t\t\t\t\t\t\t\t\t\t\t\t\t\t\tsetUploadWarningEditing(false);
\t\t\t\t\t\t\t\t\t\t\t\t\t\t}}
\t\t\t\t\t\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t\t\t\t\t\tCancel
\t\t\t\t\t\t\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t\t\t\t\t\t\t) : null}
\t\t\t\t\t\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\t\t\t\t\tvariant="outline"
\t\t\t\t\t\t\t\t\t\t\t\tclassName="h-7 text-xs"
\t\t\t\t\t\t\t\t\t\t\t\tdisabled={uploadWarningSaving}
\t\t\t\t\t\t\t\t\t\t\t\tonClick={saveUploadWarning}
\t\t\t\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t\t\t\t<Save className="h-3 w-3 mr-1" />
\t\t\t\t\t\t\t\t\t\t\t\t{uploadWarningSaving ? "Saving..." : "Save"}
\t\t\t\t\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t) : (
\t\t\t\t\t\t\t\t<div className="rounded-md border border-border bg-muted/30 px-3 py-2">
\t\t\t\t\t\t\t\t\t<p className="text-xs text-foreground whitespace-pre-wrap break-words font-mono">
\t\t\t\t\t\t\t\t\t\t{(controls.upload_warning || "").trim()}
\t\t\t\t\t\t\t\t\t</p>
\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t)}
\t\t\t\t\t\t</div>

\t\t\t\t\t\t<div className="space-y-1 pt-1">
\t\t\t\t\t\t\t<Label className="text-xs font-semibold">Upload Target Websites</Label>
\t\t\t\t\t\t\t<PolicyTargetWebsitesSelector
\t\t\t\t\t\t\t\tvalue={controls.upload_target_websites || "none"}
\t\t\t\t\t\t\t\ttargets={targets}
\t\t\t\t\t\t\t\tonChange={(val) => void patchControlWithErrorToast({ upload_target_websites: val }, "Could not save upload target websites")}
\t\t\t\t\t\t\t/>
\t\t\t\t\t\t</div>
\t\t\t\t\t</div>
\t\t\t\t)}

\t\t\t\t{/* MODE 2: Filter by File Type / Extension */}
\t\t\t\t{controls.block_file_extension && (
\t\t\t\t\t<div className="rounded-lg border border-border/70 bg-muted/15 p-4 space-y-3.5">
\t\t\t\t\t\t{/* Enforcement Mode Switcher */}
\t\t\t\t\t\t<div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 pb-2 border-b border-border/40">
\t\t\t\t\t\t\t<div className="space-y-0.5">
\t\t\t\t\t\t\t\t<div className="flex items-center gap-1.5">
\t\t\t\t\t\t\t\t\t<Label className="text-xs font-semibold">Enforcement Mode</Label>
\t\t\t\t\t\t\t\t\t{controls.file_extension_mode === "allow_listed" ? (
\t\t\t\t\t\t\t\t\t\t<Badge variant="outline" className="text-[9px] px-1.5 py-0 border-blue-500/40 text-blue-500 bg-blue-500/10">Allowlist</Badge>
\t\t\t\t\t\t\t\t\t) : (
\t\t\t\t\t\t\t\t\t\t<Badge variant="outline" className="text-[9px] px-1.5 py-0 border-amber-500/40 text-amber-500 bg-amber-500/10">Blocklist</Badge>
\t\t\t\t\t\t\t\t\t)}
\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t\t<p className="text-[11px] text-muted-foreground">
\t\t\t\t\t\t\t\t\tChoose whether to block or allow selected file extensions.
\t\t\t\t\t\t\t\t</p>
\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t<div className="flex items-center gap-2 bg-muted/30 border border-border p-1 rounded-lg text-xs shrink-0">
\t\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\t\tvariant={controls.file_extension_mode !== "allow_listed" ? "default" : "ghost"}
\t\t\t\t\t\t\t\t\tclassName="h-6 text-[11px] px-2.5 py-0"
\t\t\t\t\t\t\t\t\tonClick={() => void patchControlWithErrorToast({ file_extension_mode: "block_listed" }, "Could not save extension mode")}
\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\tBlock Selected
\t\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\t\tvariant={controls.file_extension_mode === "allow_listed" ? "default" : "ghost"}
\t\t\t\t\t\t\t\t\tclassName="h-6 text-[11px] px-2.5 py-0"
\t\t\t\t\t\t\t\t\tonClick={() => void patchControlWithErrorToast({ file_extension_mode: "allow_listed" }, "Could not save extension mode")}
\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\tAllow Selected
\t\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t</div>

\t\t\t\t\t\t{/* File Extension Warning Banner */}
\t\t\t\t\t\t<div className="space-y-1.5">
\t\t\t\t\t\t\t<div className="flex items-center justify-between">
\t\t\t\t\t\t\t\t<Label className="text-xs font-semibold">Extension Warning Message</Label>
\t\t\t\t\t\t\t\t{!fileExtensionMsgEditing && (controls.file_extension_block_message || "").trim() ? (
\t\t\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\t\t\tvariant="ghost"
\t\t\t\t\t\t\t\t\t\tclassName="h-6 text-xs text-muted-foreground hover:text-foreground"
\t\t\t\t\t\t\t\t\t\tonClick={() => {
\t\t\t\t\t\t\t\t\t\t\tsetFileExtensionMsgDraft(controls.file_extension_block_message || "");
\t\t\t\t\t\t\t\t\t\t\tsetFileExtensionMsgEditing(true);
\t\t\t\t\t\t\t\t\t\t}}
\t\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t\t<Pencil className="h-3 w-3 mr-1" />
\t\t\t\t\t\t\t\t\t\tEdit
\t\t\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t\t\t) : null}
\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t{fileExtensionMsgEditing || !(controls.file_extension_block_message || "").trim() ? (
\t\t\t\t\t\t\t\t<div className="space-y-2">
\t\t\t\t\t\t\t\t\t<Input
\t\t\t\t\t\t\t\t\t\tplaceholder="e.g. Uploading this file type is restricted."
\t\t\t\t\t\t\t\t\t\tvalue={fileExtensionMsgDraft}
\t\t\t\t\t\t\t\t\t\tonChange={(e) => setFileExtensionMsgDraft(e.target.value)}
\t\t\t\t\t\t\t\t\t\tclassName="text-xs font-sans"
\t\t\t\t\t\t\t\t\t\tonKeyDown={(e) => {
\t\t\t\t\t\t\t\t\t\t\tif (e.key === "Enter") {
\t\t\t\t\t\t\t\t\t\t\t\te.preventDefault();
\t\t\t\t\t\t\t\t\t\t\t\tvoid saveFileExtensionMsg();
\t\t\t\t\t\t\t\t\t\t\t}
\t\t\t\t\t\t\t\t\t\t}}
\t\t\t\t\t\t\t\t\t/>
\t\t\t\t\t\t\t\t\t<div className="flex items-center justify-between gap-2">
\t\t\t\t\t\t\t\t\t\t<p className="text-[11px] text-muted-foreground">
\t\t\t\t\t\t\t\t\t\t\tMessage shown when a restricted file format is blocked.
\t\t\t\t\t\t\t\t\t\t</p>
\t\t\t\t\t\t\t\t\t\t<div className="flex items-center gap-2">
\t\t\t\t\t\t\t\t\t\t\t{fileExtensionMsgEditing && (controls.file_extension_block_message || "").trim() ? (
\t\t\t\t\t\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\t\t\t\t\tvariant="ghost"
\t\t\t\t\t\t\t\t\t\t\t\tclassName="h-7 text-xs"
\t\t\t\t\t\t\t\t\t\t\t\tonClick={() => {
\t\t\t\t\t\t\t\t\t\t\t\t\tsetFileExtensionMsgDraft(controls.file_extension_block_message || "");
\t\t\t\t\t\t\t\t\t\t\t\t\tsetFileExtensionMsgEditing(false);
\t\t\t\t\t\t\t\t\t\t\t\t}}
\t\t\t\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t\t\t\tCancel
\t\t\t\t\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t\t\t\t\t) : null}
\t\t\t\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\t\t\t\tvariant="outline"
\t\t\t\t\t\t\t\t\t\t\tclassName="h-7 text-xs"
\t\t\t\t\t\t\t\t\t\t\tonClick={saveFileExtensionMsg}
\t\t\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t\t\t<Save className="h-3 w-3 mr-1" />
\t\t\t\t\t\t\t\t\t\t\tSave
\t\t\t\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t) : (
\t\t\t\t\t\t\t<div className="rounded-md border border-border bg-muted/30 px-3 py-2">
\t\t\t\t\t\t\t\t<p className="text-xs text-foreground whitespace-pre-wrap break-words font-mono">
\t\t\t\t\t\t\t\t\t{(controls.file_extension_block_message || "").trim()}
\t\t\t\t\t\t\t\t</p>
\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t)}
\t\t\t\t\t</div>

\t\t\t\t\t{/* Quick Select Buttons & Custom Adder */}
\t\t\t\t\t<div className="space-y-3 bg-muted/20 border border-border/60 rounded-lg p-3.5">
\t\t\t\t\t\t<p className="text-[11px] font-medium text-muted-foreground">
\t\t\t\t\t\t\tQuick Presets:
\t\t\t\t\t\t</p>
\t\t\t\t\t\t<div className="flex flex-wrap gap-1.5">
\t\t\t\t\t\t\t{[
\t\t\t\t\t\t\t\t{ cat: "Docs", exts: ["pdf", "docx", "xlsx", "pptx", "csv", "txt"] },
\t\t\t\t\t\t\t\t{ cat: "Code", exts: ["py", "js", "ts", "sh", "sql", "json"] },
\t\t\t\t\t\t\t\t{ cat: "Exec/Archive", exts: ["exe", "msi", "zip", "tar", "gz"] },
\t\t\t\t\t\t\t\t{ cat: "Media", exts: ["png", "jpg", "jpeg", "webp"] },
\t\t\t\t\t\t\t].map((group) => (
\t\t\t\t\t\t\t\t<div key={group.cat} className="flex items-center gap-1 border-r border-border/50 pr-2 mr-1">
\t\t\t\t\t\t\t\t\t<span className="text-[10px] text-muted-foreground font-semibold mr-0.5">{group.cat}:</span>
\t\t\t\t\t\t\t\t\t{group.exts.map((ext) => {
\t\t\t\t\t\t\t\t\t\tconst isSelected = controls.file_extension_mode === "allow_listed"
\t\t\t\t\t\t\t\t\t\t\t? configuredAllowedExts.includes(ext)
\t\t\t\t\t\t\t\t\t\t\t: configuredRestrictedExts.includes(ext);
\t\t\t\t\t\t\t\t\t\treturn (
\t\t\t\t\t\t\t\t\t\t\t<Badge
\t\t\t\t\t\t\t\t\t\t\t\tkey={ext}
\t\t\t\t\t\t\t\t\t\t\t\tvariant={isSelected ? "default" : "outline"}
\t\t\t\t\t\t\t\t\t\t\t\tclassName={`cursor-pointer text-[10px] px-2 py-0.5 transition-all select-none ${
\t\t\t\t\t\t\t\t\t\t\t\t\tisSelected
\t\t\t\t\t\t\t\t\t\t\t\t\t\t? (controls.file_extension_mode === "allow_listed" ? "bg-blue-600 text-white" : "bg-destructive text-destructive-foreground")
\t\t\t\t\t\t\t\t\t\t\t\t\t\t: "hover:bg-muted text-muted-foreground"
\t\t\t\t\t\t\t\t\t\t\t\t}`}
\t\t\t\t\t\t\t\t\t\t\t\tonClick={() => handleToggleFileExtension(ext)}
\t\t\t\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t\t\t\t.{ext}
\t\t\t\t\t\t\t\t\t\t\t</Badge>
\t\t\t\t\t\t\t\t\t\t);
\t\t\t\t\t\t\t\t\t})}
\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t))}
\t\t\t\t\t\t</div>

\t\t\t\t\t\t{/* Custom extension adder */}
\t\t\t\t\t\t<div className="flex items-center gap-2 pt-2 border-t border-border/40">
\t\t\t\t\t\t\t<Input
\t\t\t\t\t\t\t\tplaceholder="Add extension (e.g. env, pem)..."
\t\t\t\t\t\t\t\tvalue={customFileExtensionInput}
\t\t\t\t\t\t\t\tonChange={(e) => setCustomFileExtensionInput(e.target.value)}
\t\t\t\t\t\t\t\tclassName="h-7 text-xs font-mono max-w-[240px]"
\t\t\t\t\t\t\t\tonKeyDown={(e) => {
\t\t\t\t\t\t\t\t\tif (e.key === "Enter") {
\t\t\t\t\t\t\t\t\t\te.preventDefault();
\t\t\t\t\t\t\t\t\t\tvoid handleAddCustomFileExtension();
\t\t\t\t\t\t\t\t\t}
\t\t\t\t\t\t\t\t}}
\t\t\t\t\t\t\t/>
\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\tvariant="outline"
\t\t\t\t\t\t\t\tclassName="h-7 text-xs"
\t\t\t\t\t\t\t\tonClick={handleAddCustomFileExtension}
\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t+ Add
\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t</div>

\t\t\t\t\t\t{/* Active configured extensions pills */}
\t\t\t\t\t\t{((controls.file_extension_mode === "allow_listed" ? configuredAllowedExts : configuredRestrictedExts).length > 0) && (
\t\t\t\t\t\t\t<div className="flex items-center gap-1.5 flex-wrap pt-2">
\t\t\t\t\t\t\t\t<span className="text-[11px] text-muted-foreground">
\t\t\t\t\t\t\t\t\t{controls.file_extension_mode === "allow_listed" ? "Allowed:" : "Blocked:"}
\t\t\t\t\t\t\t\t</span>
\t\t\t\t\t\t\t\t{(controls.file_extension_mode === "allow_listed" ? configuredAllowedExts : configuredRestrictedExts).map((ext) => (
\t\t\t\t\t\t\t\t\t<Badge
\t\t\t\t\t\t\t\t\t\tkey={ext}
\t\t\t\t\t\t\t\t\t\tvariant="secondary"
\t\t\t\t\t\t\t\t\t\tclassName="text-[11px] font-mono px-2 py-0.5 gap-1 inline-flex items-center"
\t\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t\t.{ext}
\t\t\t\t\t\t\t\t\t\t<button
\t\t\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\t\t\tonClick={() => handleToggleFileExtension(ext)}
\t\t\t\t\t\t\t\t\t\t\tclassName="text-muted-foreground hover:text-foreground text-[10px] ml-0.5"
\t\t\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t\t\t✕
\t\t\t\t\t\t\t\t\t\t</button>
\t\t\t\t\t\t\t\t\t</Badge>
\t\t\t\t\t\t\t\t))}
\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t)}
\t\t\t\t\t</div>

\t\t\t\t\t{/* Target Websites */}
\t\t\t\t\t<div className="space-y-1 pt-1">
\t\t\t\t\t\t<PolicyTargetWebsitesSelector
\t\t\t\t\t\t\tvalue={controls.file_extension_target_websites || "none"}
\t\t\t\t\t\ttargets={targets}
\t\t\t\t\t\tonChange={(val) => void patchControlWithErrorToast({ file_extension_target_websites: val }, "Could not save file extension target websites")}
\t\t\t\t\t\t/>
\t\t\t\t\t</div>
\t\t\t\t</div>
\t\t\t)}
\t\t</div>
\t)}
</Card>

{/* POLICY 2: File Download & Export Policy */}
<Card className="border border-border bg-card overflow-hidden">
\t<div
\t\tclassName="flex items-center justify-between p-4 cursor-pointer select-none hover:bg-muted/30 transition-colors"
\t\tonClick={() => togglePolicyExpanded("download")}
\t>
\t\t<div className="flex items-center gap-3.5 min-w-0">
\t\t\t<div className="p-2.5 rounded-lg border border-border bg-muted/40 text-muted-foreground">
\t\t\t\t<HardDriveDownload className="h-5 w-5" />
\t\t\t</div>
\t\t\t<div>
\t\t\t\t<div className="flex items-center gap-2">
\t\t\t\t\t<p className="text-sm font-semibold">2. File Download &amp; Export Policy</p>
\t\t\t\t\t{controls.block_download ? (
\t\t\t\t\t\t<Badge variant="outline" className="text-[10px] px-2 py-0.5 border-emerald-500/50 text-emerald-600 dark:text-emerald-400 bg-emerald-500/10">Active</Badge>
\t\t\t\t\t) : (
\t\t\t\t\t\t<Badge variant="outline" className="text-[10px] text-muted-foreground">Disabled</Badge>
\t\t\t\t\t)}
\t\t\t\t</div>
\t\t\t\t<p className="text-xs text-muted-foreground mt-0.5">
\t\t\t\t\tBlock page export actions and browser-managed downloads on AI platforms.
\t\t\t\t</p>
\t\t\t</div>
\t\t</div>
\t\t<div className="flex items-center gap-3 shrink-0" onClick={(e) => e.stopPropagation()}>
\t\t\t<Switch
\t\t\t\tchecked={!!controls.block_download}
\t\t\t\tonCheckedChange={(val) => {
\t\t\t\t\tif (val) setExpandedPolicies((p) => ({ ...p, download: true }));
\t\t\t\t\thandleToggleDownloadGovernance(val);
\t\t\t\t}}
\t\t\t\taria-label="Toggle download blocking"
\t\t\t/>
\t\t\t<button
\t\t\t\ttype="button"
\t\t\t\tonClick={() => togglePolicyExpanded("download")}
\t\t\t\tclassName="p-1 rounded-md text-muted-foreground hover:text-foreground transition-transform"
\t\t\t>
\t\t\t\t<ChevronDown className={`h-4 w-4 transition-transform duration-200 ${expandedPolicies.download ? "rotate-180" : ""}`} />
\t\t\t</button>
\t\t</div>
\t</div>

\t{expandedPolicies.download && (
\t\t<div className="p-4 pt-0 border-t border-border/60 space-y-4 mt-2">
\t\t\t<div className="rounded-lg border border-border/70 bg-muted/15 p-4 space-y-3.5 pt-3.5">
\t\t\t\t<div className="space-y-1.5">
\t\t\t\t\t<div className="flex items-center justify-between">
\t\t\t\t\t\t<Label className="text-xs font-semibold">Download &amp; Export Warning Message</Label>
\t\t\t\t\t\t{downloadMsgEditing || !(controls.download_block_message || "").trim() ? null : (
\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\tvariant="ghost"
\t\t\t\t\t\t\t\tclassName="h-6 text-xs text-muted-foreground hover:text-foreground"
\t\t\t\t\t\t\t\tonClick={() => {
\t\t\t\t\t\t\t\t\tsetDownloadMsgDraft(controls.download_block_message || "");
\t\t\t\t\t\t\t\t\tsetDownloadMsgEditing(true);
\t\t\t\t\t\t\t\t}}
\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t<Pencil className="h-3 w-3 mr-1" />
\t\t\t\t\t\t\t\tEdit
\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t)}
\t\t\t\t\t</div>
\t\t\t\t\t{downloadMsgEditing || !(controls.download_block_message || "").trim() ? (
\t\t\t\t\t\t<div className="space-y-2">
\t\t\t\t\t\t\t<Input
\t\t\t\t\t\t\t\tplaceholder="e.g. File downloads and exports are blocked by corporate policy."
\t\t\t\t\t\t\t\tvalue={downloadMsgDraft}
\t\t\t\t\t\t\t\tonChange={(e) => setDownloadMsgDraft(e.target.value)}
\t\t\t\t\t\t\t\tclassName="text-xs font-sans"
\t\t\t\t\t\t\t\tonKeyDown={(e) => {
\t\t\t\t\t\t\t\t\tif (e.key === "Enter") {
\t\t\t\t\t\t\t\t\t\te.preventDefault();
\t\t\t\t\t\t\t\t\t\tvoid saveDownloadMsg();
\t\t\t\t\t\t\t\t\t}
\t\t\t\t\t\t\t\t}}
\t\t\t\t\t\t\t/>
\t\t\t\t\t\t\t<div className="flex items-center justify-between gap-2">
\t\t\t\t\t\t\t\t<p className="text-[11px] text-muted-foreground">
\t\t\t\t\t\t\t\t\tMessage shown when file download or export is blocked.
\t\t\t\t\t\t\t\t</p>
\t\t\t\t\t\t\t\t<div className="flex items-center gap-2">
\t\t\t\t\t\t\t\t\t{downloadMsgEditing && (controls.download_block_message || "").trim() ? (
\t\t\t\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\t\t\t\tvariant="ghost"
\t\t\t\t\t\t\t\t\t\t\tclassName="h-7 text-xs"
\t\t\t\t\t\t\t\t\t\t\tonClick={() => {
\t\t\t\t\t\t\t\t\t\t\t\tsetDownloadMsgDraft(controls.download_block_message || "");
\t\t\t\t\t\t\t\t\t\t\t\tsetDownloadMsgEditing(false);
\t\t\t\t\t\t\t\t\t\t\t}}
\t\t\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t\t\tCancel
\t\t\t\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t\t\t\t) : null}
\t\t\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\t\t\tvariant="outline"
\t\t\t\t\t\t\t\t\t\tclassName="h-7 text-xs"
\t\t\t\t\t\t\t\t\t\tonClick={saveDownloadMsg}
\t\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t\t<Save className="h-3 w-3 mr-1" />
\t\t\t\t\t\t\t\t\t\tSave
\t\t\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t</div>
\t\t\t\t\t) : (
\t\t\t\t\t\t<div className="rounded-md border border-border bg-muted/30 px-3 py-2">
\t\t\t\t\t\t\t<p className="text-xs text-foreground whitespace-pre-wrap break-words font-mono">
\t\t\t\t\t\t\t\t{(controls.download_block_message || "").trim()}
\t\t\t\t\t\t\t</p>
\t\t\t\t\t\t</div>
\t\t\t\t\t)}
\t\t\t\t</div>

\t\t\t\t<div className="space-y-1 pt-1">
\t\t\t\t\t<Label className="text-xs font-semibold">Download Target Websites</Label>
\t\t\t\t\t<PolicyTargetWebsitesSelector
\t\t\t\t\t\tvalue={controls.download_target_websites || "none"}
\t\t\t\t\t\ttargets={targets}
\t\t\t\t\t\tonChange={(val) => void patchControlWithErrorToast({ download_target_websites: val }, "Could not save download target websites")}
\t\t\t\t\t/>
\t\t\t\t</div>
\t\t\t</div>
\t\t</div>
\t)}
</Card>

{/* POLICY 3: Drag-and-Drop Ingestion Blocking */}
<Card className="border border-border bg-card overflow-hidden">
\t<div
\t\tclassName="flex items-center justify-between p-4 cursor-pointer select-none hover:bg-muted/30 transition-colors"
\t\tonClick={() => togglePolicyExpanded("drag_drop")}
\t>
\t\t<div className="flex items-center gap-3.5 min-w-0">
\t\t\t<div className="p-2.5 rounded-lg border border-border bg-muted/40 text-muted-foreground">
\t\t\t\t<FileSpreadsheet className="h-5 w-5" />
\t\t\t</div>
\t\t\t<div>
\t\t\t\t<div className="flex items-center gap-2">
\t\t\t\t\t<p className="text-sm font-semibold">3. Drag &amp; Drop Blocking</p>
\t\t\t\t\t{controls.block_drag_drop ? (
\t\t\t\t\t\t<Badge variant="outline" className="text-[10px] px-2 py-0.5 border-emerald-500/50 text-emerald-600 dark:text-emerald-400 bg-emerald-500/10">Active</Badge>
\t\t\t\t\t) : (
\t\t\t\t\t\t<Badge variant="outline" className="text-[10px] text-muted-foreground">Disabled</Badge>
\t\t\t\t\t)}
\t\t\t\t</div>
\t\t\t\t<p className="text-xs text-muted-foreground mt-0.5">
\t\t\t\t\tBlock dragging and dropping files or text into AI tools.
\t\t\t\t</p>
\t\t\t</div>
\t\t</div>
\t\t<div className="flex items-center gap-3 shrink-0" onClick={(e) => e.stopPropagation()}>
\t\t\t<Switch
\t\t\t\tchecked={!!controls.block_drag_drop}
\t\t\t\tonCheckedChange={(val) => {
\t\t\t\t\tif (val) setExpandedPolicies((p) => ({ ...p, drag_drop: true }));
\t\t\t\t\thandleToggleDragDropGovernance(val);
\t\t\t\t}}
\t\t\t\taria-label="Toggle Drag-and-Drop Blocking"
\t\t\t/>
\t\t\t<button
\t\t\t\ttype="button"
\t\t\t\tonClick={() => togglePolicyExpanded("drag_drop")}
\t\t\t\tclassName="p-1 rounded-md text-muted-foreground hover:text-foreground transition-transform"
\t\t\t>
\t\t\t\t<ChevronDown className={`h-4 w-4 transition-transform duration-200 ${expandedPolicies.drag_drop ? "rotate-180" : ""}`} />
\t\t\t</button>
\t\t</div>
\t</div>

\t{expandedPolicies.drag_drop && (
\t\t<div className="p-4 pt-3 border-t border-border/60 space-y-3">
\t\t\t<div className="space-y-1.5">
\t\t\t\t<div className="flex items-center justify-between">
\t\t\t\t\t<p className="text-xs font-semibold text-foreground">Warning Message</p>
\t\t\t\t\t{dragDropMsgEditing || !(controls.drag_drop_block_message || "").trim() ? null : (
\t\t\t\t\t\t<Button
\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\tvariant="ghost"
\t\t\t\t\t\t\tclassName="h-6 text-xs text-muted-foreground hover:text-foreground"
\t\t\t\t\t\t\tonClick={() => {
\t\t\t\t\t\t\t\tsetDragDropMsgDraft(controls.drag_drop_block_message || "");
\t\t\t\t\t\t\t\tsetDragDropMsgEditing(true);
\t\t\t\t\t\t\t}}
\t\t\t\t\t\t>
\t\t\t\t\t\t\tEdit
\t\t\t\t\t\t</Button>
\t\t\t\t\t)}
\t\t\t\t</div>
\t\t\t\t{dragDropMsgEditing || !(controls.drag_drop_block_message || "").trim() ? (
\t\t\t\t\t<div className="space-y-2">
\t\t\t\t\t\t<Input
\t\t\t\t\t\t\tplaceholder="e.g. File drag and drop is restricted by corporate policy."
\t\t\t\t\t\t\tvalue={dragDropMsgDraft}
\t\t\t\t\t\t\tonChange={(e) => setDragDropMsgDraft(e.target.value)}
\t\t\t\t\t\t\tclassName="text-xs font-sans"
\t\t\t\t\t\t\tonKeyDown={(e) => {
\t\t\t\t\t\t\t\tif (e.key === "Enter") {
\t\t\t\t\t\t\t\t\te.preventDefault();
\t\t\t\t\t\t\t\t\tvoid saveDragDropMsg();
\t\t\t\t\t\t\t\t}
\t\t\t\t\t\t\t}}
\t\t\t\t\t\t/>
\t\t\t\t\t\t<div className="flex items-center justify-between gap-2">
\t\t\t\t\t\t\t<p className="text-[11px] text-muted-foreground">
\t\t\t\t\t\t\t\tMessage shown when drag and drop is blocked.
\t\t\t\t\t\t\t</p>
\t\t\t\t\t\t\t<div className="flex items-center gap-2">
\t\t\t\t\t\t\t\t{dragDropMsgEditing && (controls.drag_drop_block_message || "").trim() ? (
\t\t\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\t\t\tvariant="ghost"
\t\t\t\t\t\t\t\t\t\tclassName="h-7 text-xs"
\t\t\t\t\t\t\t\t\t\tonClick={() => {
\t\t\t\t\t\t\t\t\t\t\tsetDragDropMsgDraft(controls.drag_drop_block_message || "");
\t\t\t\t\t\t\t\t\t\t\tsetDragDropMsgEditing(false);
\t\t\t\t\t\t\t\t\t\t}}
\t\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t\tCancel
\t\t\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t\t\t) : null}
\t\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\t\tvariant="outline"
\t\t\t\t\t\t\t\t\tclassName="h-7 text-xs"
\t\t\t\t\t\t\t\t\tonClick={saveDragDropMsg}
\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t<Save className="h-3 w-3 mr-1" />
\t\t\t\t\t\t\t\t\tSave
\t\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t</div>
\t\t\t\t\t</div>
\t\t\t\t) : (
\t\t\t\t\t<div className="rounded-md border border-border bg-muted/30 px-3 py-2">
\t\t\t\t\t\t<p className="text-xs text-foreground whitespace-pre-wrap break-words font-mono">
\t\t\t\t\t\t\t{(controls.drag_drop_block_message || "").trim()}
\t\t\t\t\t\t</p>
\t\t\t\t\t</div>
\t\t\t\t)}
\t\t\t</div>

\t\t\t<PolicyTargetWebsitesSelector
\t\t\t\tvalue={controls.drag_drop_target_websites || "none"}
\t\t\t\ttargets={targets}
\t\t\t\tonChange={(val) => void patchControlWithErrorToast({ drag_drop_target_websites: val }, "Could not save target websites")}
\t\t\t/>
\t\t</div>
\t)}
</Card>

{/* POLICY 4: Clipboard Restriction (Copy & Paste) */}
<Card className="border border-border bg-card overflow-hidden">
\t<div
\t\tclassName="flex items-center justify-between p-4 cursor-pointer select-none hover:bg-muted/30 transition-colors"
\t\tonClick={() => togglePolicyExpanded("clipboard")}
\t>
\t\t<div className="flex items-center gap-3.5 min-w-0">
\t\t\t<div className="p-2.5 rounded-lg border border-border bg-muted/40 text-muted-foreground">
\t\t\t\t<Copy className="h-5 w-5" />
\t\t\t</div>
\t\t\t<div>
\t\t\t\t<div className="flex items-center gap-2">
\t\t\t\t\t<p className="text-sm font-semibold">4. Clipboard Restriction (Copy &amp; Paste)</p>
\t\t\t\t\t{controls.block_clipboard ? (
\t\t\t\t\t\t<Badge variant="outline" className="text-[10px] px-2 py-0.5 border-emerald-500/50 text-emerald-600 dark:text-emerald-400 bg-emerald-500/10">Active</Badge>
\t\t\t\t\t) : (
\t\t\t\t\t\t<Badge variant="outline" className="text-[10px] text-muted-foreground">Disabled</Badge>
\t\t\t\t\t)}
\t\t\t\t</div>
\t\t\t\t<p className="text-xs text-muted-foreground mt-0.5">
\t\t\t\t\tBlock unauthorized clipboard copying and pasting on monitored AI tools (restricts both copying from AI outputs and pasting into AI inputs).
\t\t\t\t</p>
\t\t\t</div>
\t\t</div>
\t\t<div className="flex items-center gap-3 shrink-0" onClick={(e) => e.stopPropagation()}>
\t\t\t<Switch
\t\t\t\tchecked={!!controls.block_clipboard}
\t\t\t\tonCheckedChange={(val) => {
\t\t\t\t\tif (val) setExpandedPolicies((p) => ({ ...p, clipboard: true }));
\t\t\t\t\thandleToggleClipboardGovernance(val);
\t\t\t\t}}
\t\t\t\taria-label="Toggle Copy-Paste Blocking"
\t\t\t/>
\t\t\t<button
\t\t\t\ttype="button"
\t\t\t\tonClick={() => togglePolicyExpanded("clipboard")}
\t\t\t\tclassName="p-1 rounded-md text-muted-foreground hover:text-foreground transition-transform"
\t\t\t>
\t\t\t\t<ChevronDown className={`h-4 w-4 transition-transform duration-200 ${expandedPolicies.clipboard ? "rotate-180" : ""}`} />
\t\t\t</button>
\t\t</div>
\t</div>

\t{expandedPolicies.clipboard && (
\t\t<div className="p-4 pt-3 border-t border-border/60 space-y-3">
\t\t\t<div className="space-y-1.5">
\t\t\t\t<div className="flex items-center justify-between">
\t\t\t\t\t<p className="text-xs font-semibold text-foreground">Warning Message</p>
\t\t\t\t\t{clipboardMsgEditing || !(controls.clipboard_block_message || "").trim() ? null : (
\t\t\t\t\t\t<Button
\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\tvariant="ghost"
\t\t\t\t\t\t\tclassName="h-6 text-xs text-muted-foreground hover:text-foreground"
\t\t\t\t\t\t\tonClick={() => {
\t\t\t\t\t\t\t\tsetClipboardMsgDraft(controls.clipboard_block_message || "");
\t\t\t\t\t\t\t\tsetClipboardMsgEditing(true);
\t\t\t\t\t\t\t}}
\t\t\t\t\t\t>
\t\t\t\t\t\t\tEdit
\t\t\t\t\t\t</Button>
\t\t\t\t\t)}
\t\t\t\t</div>
\t\t\t\t{clipboardMsgEditing || !(controls.clipboard_block_message || "").trim() ? (
\t\t\t\t\t<div className="space-y-2">
\t\t\t\t\t\t<Input
\t\t\t\t\t\t\tplaceholder="e.g. Clipboard copy-paste is restricted by corporate policy."
\t\t\t\t\t\t\tvalue={clipboardMsgDraft}
\t\t\t\t\t\t\tonChange={(e) => setClipboardMsgDraft(e.target.value)}
\t\t\t\t\t\t\tclassName="text-xs font-sans"
\t\t\t\t\t\t\tonKeyDown={(e) => {
\t\t\t\t\t\t\t\tif (e.key === "Enter") {
\t\t\t\t\t\t\t\t\te.preventDefault();
\t\t\t\t\t\t\t\t\tvoid saveClipboardMsg();
\t\t\t\t\t\t\t\t}
\t\t\t\t\t\t\t}}
\t\t\t\t\t\t/>
\t\t\t\t\t\t<div className="flex items-center justify-between gap-2">
\t\t\t\t\t\t\t<p className="text-[11px] text-muted-foreground">
\t\t\t\t\t\t\t\tMessage shown when copy or paste is blocked.
\t\t\t\t\t\t\t</p>
\t\t\t\t\t\t\t<div className="flex items-center gap-2">
\t\t\t\t\t\t\t\t{clipboardMsgEditing && (controls.clipboard_block_message || "").trim() ? (
\t\t\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\t\t\tvariant="ghost"
\t\t\t\t\t\t\t\t\t\tclassName="h-7 text-xs"
\t\t\t\t\t\t\t\t\t\tonClick={() => {
\t\t\t\t\t\t\t\t\t\t\tsetClipboardMsgDraft(controls.clipboard_block_message || "");
\t\t\t\t\t\t\t\t\t\t\tsetClipboardMsgEditing(false);
\t\t\t\t\t\t\t\t\t\t}}
\t\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t\tCancel
\t\t\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t\t\t) : null}
\t\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\t\tvariant="outline"
\t\t\t\t\t\t\t\t\tclassName="h-7 text-xs"
\t\t\t\t\t\t\t\t\tonClick={saveClipboardMsg}
\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t<Save className="h-3 w-3 mr-1" />
\t\t\t\t\t\t\t\t\tSave
\t\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t</div>
\t\t\t\t\t</div>
\t\t\t\t) : (
\t\t\t\t\t<div className="rounded-md border border-border bg-muted/30 px-3 py-2">
\t\t\t\t\t\t<p className="text-xs text-foreground whitespace-pre-wrap break-words font-mono">
\t\t\t\t\t\t\t{(controls.clipboard_block_message || "").trim()}
\t\t\t\t\t\t</p>
\t\t\t\t\t</div>
\t\t\t\t)}
\t\t\t</div>

\t\t\t<PolicyTargetWebsitesSelector
\t\t\t\tvalue={controls.clipboard_target_websites || "none"}
\t\t\t\ttargets={targets}
\t\t\t\tonChange={(val) => void patchControlWithErrorToast({ clipboard_target_websites: val, governance_target_websites: val }, "Could not save target websites")}
\t\t\t/>
\t\t</div>
\t)}
</Card>

{/* POLICY 5: Screenshot Capture Deterrent */}
<Card className="border border-border bg-card overflow-hidden">
\t<div
\t\tclassName="flex items-center justify-between p-4 cursor-pointer select-none hover:bg-muted/30 transition-colors"
\t\tonClick={() => togglePolicyExpanded("screenshot")}
\t>
\t\t<div className="flex items-center gap-3.5 min-w-0">
\t\t\t<div className="p-2.5 rounded-lg border border-border bg-muted/40 text-muted-foreground">
\t\t\t\t<Camera className="h-5 w-5" />
\t\t\t</div>
\t\t\t<div>
\t\t\t\t<div className="flex items-center gap-2">
\t\t\t\t\t<p className="text-sm font-semibold">5. Screenshot Deterrent</p>
\t\t\t\t\t{controls.block_screenshot ? (
\t\t\t\t\t\t<Badge variant="outline" className="text-[10px] px-2 py-0.5 border-emerald-500/50 text-emerald-600 dark:text-emerald-400 bg-emerald-500/10">Active</Badge>
\t\t\t\t\t) : (
\t\t\t\t\t\t<Badge variant="outline" className="text-[10px] text-muted-foreground">Disabled</Badge>
\t\t\t\t\t)}
\t\t\t\t</div>
\t\t\t\t<p className="text-xs text-muted-foreground mt-0.5">
\t\t\t\t\tBlock print and screenshot capture shortcuts on monitored AI tools.
\t\t\t\t</p>
\t\t\t</div>
\t\t</div>
\t\t<div className="flex items-center gap-3 shrink-0" onClick={(e) => e.stopPropagation()}>
\t\t\t<Switch
\t\t\t\tchecked={!!controls.block_screenshot}
\t\t\t\tonCheckedChange={(val) => {
\t\t\t\t\tif (val) setExpandedPolicies((p) => ({ ...p, screenshot: true }));
\t\t\t\t\thandleToggleScreenshotProtection(val);
\t\t\t\t}}
\t\t\t\taria-label="Toggle screenshot capture deterrent"
\t\t\t/>
\t\t\t<button
\t\t\t\ttype="button"
\t\t\t\tonClick={() => togglePolicyExpanded("screenshot")}
\t\t\t\tclassName="p-1 rounded-md text-muted-foreground hover:text-foreground transition-transform"
\t\t\t>
\t\t\t\t<ChevronDown className={`h-4 w-4 transition-transform duration-200 ${expandedPolicies.screenshot ? "rotate-180" : ""}`} />
\t\t\t</button>
\t\t</div>
\t</div>

\t{expandedPolicies.screenshot && (
\t\t<div className="p-4 pt-3 border-t border-border/60 space-y-3">
\t\t\t<div className="space-y-1.5">
\t\t\t\t<div className="flex items-center justify-between">
\t\t\t\t\t<p className="text-xs font-semibold text-foreground">Warning Message</p>
\t\t\t\t\t{screenshotMsgEditing || !(controls.screenshot_block_message || "").trim() ? null : (
\t\t\t\t\t\t<Button
\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\tvariant="ghost"
\t\t\t\t\t\t\tclassName="h-6 text-xs text-muted-foreground hover:text-foreground"
\t\t\t\t\t\t\tonClick={() => {
\t\t\t\t\t\t\t\tsetScreenshotMsgDraft(controls.screenshot_block_message || "");
\t\t\t\t\t\t\t\tsetScreenshotMsgEditing(true);
\t\t\t\t\t\t\t}}
\t\t\t\t\t\t>
\t\t\t\t\t\t\tEdit
\t\t\t\t\t\t</Button>
\t\t\t\t\t)}
\t\t\t\t</div>
\t\t\t\t{screenshotMsgEditing || !(controls.screenshot_block_message || "").trim() ? (
\t\t\t\t\t<div className="space-y-2">
\t\t\t\t\t\t<Input
\t\t\t\t\t\t\tplaceholder="e.g. Screen capture is restricted by corporate policy."
\t\t\t\t\t\t\tvalue={screenshotMsgDraft}
\t\t\t\t\t\t\tonChange={(e) => setScreenshotMsgDraft(e.target.value)}
\t\t\t\t\t\t\tclassName="text-xs font-sans"
\t\t\t\t\t\t\tonKeyDown={(e) => {
\t\t\t\t\t\t\t\tif (e.key === "Enter") {
\t\t\t\t\t\t\t\t\te.preventDefault();
\t\t\t\t\t\t\t\t\tvoid saveScreenshotMsg();
\t\t\t\t\t\t\t\t}
\t\t\t\t\t\t\t}}
\t\t\t\t\t\t/>
\t\t\t\t\t\t<div className="flex items-center justify-between gap-2">
\t\t\t\t\t\t\t<p className="text-[11px] text-muted-foreground">
\t\t\t\t\t\t\t\tMessage shown when screen capture shortcut is detected.
\t\t\t\t\t\t\t</p>
\t\t\t\t\t\t\t<div className="flex items-center gap-2">
\t\t\t\t\t\t\t\t{screenshotMsgEditing && (controls.screenshot_block_message || "").trim() ? (
\t\t\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\t\t\tvariant="ghost"
\t\t\t\t\t\t\t\t\t\tclassName="h-7 text-xs"
\t\t\t\t\t\t\t\t\t\tonClick={() => {
\t\t\t\t\t\t\t\t\t\t\tsetScreenshotMsgDraft(controls.screenshot_block_message || "");
\t\t\t\t\t\t\t\t\t\t\tsetScreenshotMsgEditing(false);
\t\t\t\t\t\t\t\t\t\t}}
\t\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t\tCancel
\t\t\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t\t\t) : null}
\t\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\t\tvariant="outline"
\t\t\t\t\t\t\t\t\tclassName="h-7 text-xs"
\t\t\t\t\t\t\t\t\tonClick={saveScreenshotMsg}
\t\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t\t<Save className="h-3 w-3 mr-1" />
\t\t\t\t\t\t\t\t\tSave
\t\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t\t</div>
\t\t\t\t\t\t</div>
\t\t\t\t\t</div>
\t\t\t\t) : (
\t\t\t\t\t<div className="rounded-md border border-border bg-muted/30 px-3 py-2">
\t\t\t\t\t\t<p className="text-xs text-foreground whitespace-pre-wrap break-words font-mono">
\t\t\t\t\t\t\t{(controls.screenshot_block_message || "").trim()}
\t\t\t\t\t\t</p>
\t\t\t\t\t</div>
\t\t\t\t)}
\t\t\t</div>

\t\t\t<PolicyTargetWebsitesSelector
\t\t\t\tvalue={controls.screenshot_target_websites || "none"}
\t\t\t\ttargets={targets}
\t\t\t\tonChange={(val) => void patchControlWithErrorToast({ screenshot_target_websites: val }, "Could not save target websites")}
\t\t\t/>
\t\t</div>
\t)}
</Card>

{/* POLICY 6: Temporary File Storage & Retention Policy */}
<Card className="border border-border bg-card overflow-hidden">
\t<div
\t\tclassName="flex items-center justify-between p-4 cursor-pointer select-none hover:bg-muted/30 transition-colors"
\t\tonClick={() => togglePolicyExpanded("retention")}
\t>
\t\t<div className="flex items-center gap-3.5 min-w-0">
\t\t\t<div className="p-2.5 rounded-lg border border-border bg-muted/40 text-muted-foreground">
\t\t\t\t<Clock className="h-5 w-5" />
\t\t\t</div>
\t\t\t<div>
\t\t\t\t<div className="flex items-center gap-2">
\t\t\t\t\t<p className="text-sm font-semibold">6. File Storage &amp; Retention</p>
\t\t\t\t\t<Badge variant="outline" className="text-[10px] px-2 py-0.5 border-emerald-500/50 text-emerald-600 dark:text-emerald-400 bg-emerald-500/10">
\t\t\t\t\t\tAuto-Purge: {controls.attachment_retention || "1h"}
\t\t\t\t\t</Badge>
\t\t\t\t</div>
\t\t\t\t<p className="text-xs text-muted-foreground mt-0.5">
\t\t\t\t\tRetention period for temporary uploaded files on disk. Metadata in logs is kept permanently.
\t\t\t\t</p>
\t\t\t</div>
\t\t</div>
\t\t<div className="flex items-center gap-3 shrink-0" onClick={(e) => e.stopPropagation()}>
\t\t\t<button
\t\t\t\ttype="button"
\t\t\t\tonClick={() => togglePolicyExpanded("retention")}
\t\t\t\tclassName="p-1 rounded-md text-muted-foreground hover:text-foreground transition-transform"
\t\t\t>
\t\t\t\t<ChevronDown className={`h-4 w-4 transition-transform duration-200 ${expandedPolicies.retention ? "rotate-180" : ""}`} />
\t\t\t</button>
\t\t</div>
\t</div>

\t{expandedPolicies.retention && (
\t\t<div className="p-4 pt-0 border-t border-border/60 space-y-4 mt-2">
\t\t\t<div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pt-3">
\t\t\t\t<div className="space-y-0.5">
\t\t\t\t\t<div className="flex items-center gap-1.5">
\t\t\t\t\t\t<Label className="text-xs font-semibold">Storage Retention Period</Label>
\t\t\t\t\t\t<Badge variant="outline" className="text-[9px] px-1.5 py-0 border-emerald-500/40 text-emerald-600 dark:text-emerald-400">Disk auto-purge</Badge>
\t\t\t\t\t</div>
\t\t\t\t\t<p className="text-[11px] text-muted-foreground">
\t\t\t\t\t\tSet how long cached files stay on disk before auto-purge.
\t\t\t\t\t</p>
\t\t\t\t</div>
\t\t\t\t<Select
\t\t\t\t\tvalue={["10m", "1h", "2h", "1d", "7d"].includes(controls.attachment_retention || "") ? controls.attachment_retention : "1h"}
\t\t\t\t\tonValueChange={(val) => void patchControlWithErrorToast({ attachment_retention: val }, "Could not save attachment retention")}
\t\t\t\t>
\t\t\t\t\t<SelectTrigger className="h-8 w-[160px] text-xs">
\t\t\t\t\t\t<SelectValue />
\t\t\t\t\t</SelectTrigger>
\t\t\t\t\t<SelectContent>
\t\t\t\t\t\t<SelectItem value="10m">10 minutes</SelectItem>
\t\t\t\t\t\t<SelectItem value="1h">1 hour (Default)</SelectItem>
\t\t\t\t\t\t<SelectItem value="2h">2 hours</SelectItem>
\t\t\t\t\t\t<SelectItem value="1d">1 day (24h)</SelectItem>
\t\t\t\t\t\t<SelectItem value="7d">7 days (1w)</SelectItem>
\t\t\t\t\t</SelectContent>
\t\t\t\t</Select>
\t\t\t</div>

\t\t\t{/* Quick Select Retention Presets */}
\t\t\t<div className="space-y-2.5 bg-muted/20 border border-border/60 rounded-lg p-3.5">
\t\t\t\t<p className="text-[11px] font-medium text-muted-foreground">
\t\t\t\t\tPresets:
\t\t\t\t</p>
\t\t\t\t<div className="flex flex-wrap gap-2">
\t\t\t\t\t{[
\t\t\t\t\t\t{ label: "10 Minutes", val: "10m", badge: "Shortest" },
\t\t\t\t\t\t{ label: "1 Hour", val: "1h", badge: "Default" },
\t\t\t\t\t\t{ label: "2 Hours", val: "2h", badge: "Standard" },
\t\t\t\t\t\t{ label: "1 Day (24h)", val: "1d", badge: "Extended" },
\t\t\t\t\t\t{ label: "7 Days (1w)", val: "7d", badge: "Weekly" },
\t\t\t\t\t].map((item) => {
\t\t\t\t\t\tconst isSelected = (controls.attachment_retention || "1h") === item.val;
\t\t\t\t\t\treturn (
\t\t\t\t\t\t\t<Button
\t\t\t\t\t\t\t\tkey={item.val}
\t\t\t\t\t\t\t\ttype="button"
\t\t\t\t\t\t\t\tsize="sm"
\t\t\t\t\t\t\t\tvariant={isSelected ? "default" : "outline"}
\t\t\t\t\t\t\t\tclassName={`h-7 text-xs px-3 transition-all ${
\t\t\t\t\t\t\t\t\tisSelected
\t\t\t\t\t\t\t\t\t\t? "bg-primary text-primary-foreground font-semibold"
\t\t\t\t\t\t\t\t\t\t: "hover:bg-muted text-muted-foreground"
\t\t\t\t\t\t\t\t}`}
\t\t\t\t\t\t\t\tonClick={() => void patchControlWithErrorToast({ attachment_retention: item.val }, "Could not save attachment retention")}
\t\t\t\t\t\t\t>
\t\t\t\t\t\t\t\t{item.label}
\t\t\t\t\t\t\t\t{item.badge ? (
\t\t\t\t\t\t\t\t\t<span className="ml-1 opacity-70 text-[10px]">({item.badge})</span>
\t\t\t\t\t\t\t\t) : null}
\t\t\t\t\t\t\t</Button>
\t\t\t\t\t\t);
\t\t\t\t\t})}
\t\t\t\t</div>
\t\t\t\t<div className="rounded border border-border/50 bg-background/50 p-2.5 text-[11px] text-muted-foreground space-y-1">
\t\t\t\t\t<p className="font-medium text-foreground">Data Compliance:</p>
\t\t\t\t\t<p>
\t\t\t\t\t\tExpired files are shredded from disk. File metadata, hashes, and audit logs are preserved permanently.
\t\t\t\t\t</p>
\t\t\t\t</div>
\t\t\t</div>
\t\t</div>
\t)}
</Card>
'''

def main():
    with open(r'ui/app/workspace/browser-ai/page.tsx', 'r', encoding='utf-8') as f:
        content = f.read()

    start_marker = '\t\t\t\t\t\t{/* POLICY 1: File Upload & Download Policy */}'
    end_marker = '\t\t\t\t\t\t</Card>\n\t\t\t\t\t</div>\n\t\t\t\t</TabsContent>'

    idx_start = content.find(start_marker)
    idx_end = content.find(end_marker, idx_start)

    if idx_start == -1 or idx_end == -1:
        print(f"Error: markers not found (start={idx_start}, end={idx_end})")
        return

    idx_end_card = idx_end + len('\t\t\t\t\t\t</Card>\n')
    new_content = content[:idx_start] + new_cards + content[idx_end_card:]

    with open(r'ui/app/workspace/browser-ai/page.tsx', 'w', encoding='utf-8') as f:
        f.write(new_content)
    print("SUCCESS: page.tsx successfully updated!")

if __name__ == "__main__":
    main()
