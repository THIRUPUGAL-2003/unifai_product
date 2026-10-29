import { type MessageContent } from "@/lib/message";
import { toast } from "sonner";
import { audioFormatFromMimeOrName, normalizeAudioToWavFile } from "./audioNormalize";
import { extractPromptFileText } from "./extractFileText";
import { extractArchiveInnerFiles, isPromptArchive } from "./extractArchive";
import { isThinExtractedText, ocrImageFile, ocrPdfFile, ocrTextAttachment } from "./ocrImage";
import { MAX_AUDIO_TRANSCRIPTION_BYTES, transcribeAudioFile, voiceTranscriptAttachment } from "./transcribeAudio";

/** Accepted file types for prompt repository attachments */
export const PROMPT_FILE_ACCEPT = "*/*";

export const PROMPT_FILE_ACCEPT_LABEL =
	"All file types: Videos (MP4, WEBM, MOV, MKV, AVI, M4V), Images (PNG, JPG, WEBP, GIF, SVG), Code, PDF, Word, Excel, PowerPoint, Zip archives, Audio, and Text files";

export const MAX_PROMPT_ATTACHMENT_BYTES = 2048 * 1024 * 1024; // 2 GB (supports large files and media)

const EXTENSION_MIME: Record<string, string> = {
	// 1. Documents and Office Files
	pdf: "application/pdf",
	doc: "application/msword",
	docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	dot: "application/msword",
	dotx: "application/vnd.openxmlformats-officedocument.wordprocessingml.template",
	dotm: "application/vnd.ms-word.template.macroEnabled.12",
	odt: "application/vnd.oasis.opendocument.text",
	rtf: "application/rtf",
	txt: "text/plain",
	md: "text/markdown",
	markdown: "text/markdown",
	mdown: "text/markdown",
	tex: "text/x-tex",
	latex: "text/x-latex",
	xls: "application/vnd.ms-excel",
	xlsx: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	xlsm: "application/vnd.ms-excel.sheet.macroEnabled.12",
	xlsb: "application/vnd.ms-excel.sheet.binary.macroEnabled.12",
	xlt: "application/vnd.ms-excel",
	xltx: "application/vnd.openxmlformats-officedocument.spreadsheetml.template",
	xltm: "application/vnd.ms-excel.template.macroEnabled.12",
	csv: "text/csv",
	tsv: "text/tab-separated-values",
	ods: "application/vnd.oasis.opendocument.spreadsheet",
	numbers: "application/vnd.apple.numbers",
	ppt: "application/vnd.ms-powerpoint",
	pptx: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	pptm: "application/vnd.ms-powerpoint.presentation.macroEnabled.12",
	pot: "application/vnd.ms-powerpoint",
	potx: "application/vnd.openxmlformats-officedocument.presentationml.template",
	potm: "application/vnd.ms-powerpoint.template.macroEnabled.12",
	odp: "application/vnd.oasis.opendocument.presentation",
	key: "application/vnd.apple.keynote",
	pub: "application/x-mspublisher",
	vsd: "application/vnd.visio",
	vsdx: "application/vnd.visio",
	xps: "application/vnd.ms-xpsdocument",
	oxps: "application/oxps",
	epub: "application/epub+zip",
	azw: "application/vnd.amazon.ebook",
	azw3: "application/vnd.amazon.ebook",
	kfx: "application/vnd.amazon.ebook",
	fb2: "application/x-fictionbook+xml",
	mobi: "application/x-mobipocket-ebook",
	djvu: "image/vnd.djvu",
	djv: "image/vnd.djvu",
	ps: "application/postscript",
	eps: "application/postscript",
	cbz: "application/vnd.comicbook+zip",
	cbr: "application/vnd.comicbook-rar",

	// 2. Programming and Source Code Files
	py: "text/x-python",
	pyw: "text/x-python",
	pyi: "text/x-python",
	pyx: "text/x-python",
	pyc: "application/x-python-code",
	pyd: "application/x-python-code",
	js: "text/javascript",
	mjs: "text/javascript",
	cjs: "text/javascript",
	ts: "text/typescript",
	tsx: "text/typescript",
	mts: "text/typescript",
	cts: "text/typescript",
	html: "text/html",
	htm: "text/html",
	css: "text/css",
	sass: "text/x-sass",
	scss: "text/x-scss",
	less: "text/x-less",
	java: "text/x-java-source",
	class: "application/java-vm",
	jar: "application/java-archive",
	war: "application/java-archive",
	c: "text/x-c",
	h: "text/x-c",
	cpp: "text/x-c++",
	cc: "text/x-c++",
	cxx: "text/x-c++",
	hpp: "text/x-c++",
	hh: "text/x-c++",
	hxx: "text/x-c++",
	cs: "text/x-csharp",
	csx: "text/x-csharp",
	fs: "text/x-fsharp",
	fsi: "text/x-fsharp",
	fsx: "text/x-fsharp",
	go: "text/x-go",
	rs: "text/x-rust",
	rlib: "application/octet-stream",
	php: "text/x-php",
	phtml: "text/x-php",
	phar: "application/octet-stream",
	rb: "text/x-ruby",
	rake: "text/x-ruby",
	gemspec: "text/x-ruby",
	swift: "text/x-swift",
	kt: "text/x-kotlin",
	kts: "text/x-kotlin",
	dart: "text/plain",
	r: "text/x-r",
	rmd: "text/x-r",
	rds: "application/octet-stream",
	rdata: "application/octet-stream",
	m: "text/plain",
	mlx: "text/plain",
	mat: "application/octet-stream",
	jl: "text/plain",
	scala: "text/x-scala",
	sc: "text/x-scala",
	pl: "text/x-perl",
	pm: "text/x-perl",
	t: "text/x-perl",
	lua: "text/plain",
	sh: "text/x-shellscript",
	bash: "text/x-shellscript",
	zsh: "text/x-shellscript",
	fish: "text/x-shellscript",
	bat: "text/plain",
	cmd: "text/plain",
	ps1: "text/x-powershell",
	psm1: "text/x-powershell",
	psd1: "text/x-powershell",
	asm: "text/plain",
	s: "text/plain",
	f: "text/x-fortran",
	for: "text/x-fortran",
	f90: "text/x-fortran",
	f95: "text/x-fortran",
	cbl: "text/plain",
	cob: "text/plain",
	cpy: "text/plain",
	mm: "text/x-c++",
	ex: "text/plain",
	exs: "text/plain",
	erl: "text/plain",
	hrl: "text/plain",
	hs: "text/x-haskell",
	lhs: "text/x-haskell",
	clj: "text/x-clojure",
	cljs: "text/x-clojure",
	cljc: "text/x-clojure",
	edn: "text/plain",
	lisp: "text/plain",
	lsp: "text/plain",
	cl: "text/plain",
	pro: "text/plain",
	sol: "text/plain",
	sql: "text/x-sql",
	graphql: "text/plain",
	gql: "text/plain",
	proto: "text/plain",
	wasm: "application/wasm",
	wat: "text/plain",
	ipynb: "application/x-ipynb+json",

	// 3. Web Development, Configuration and Project Files
	jsx: "text/javascript",
	vue: "text/plain",
	svelte: "text/plain",
	aspx: "text/plain",
	ascx: "text/plain",
	cshtml: "text/plain",
	razor: "text/plain",
	manifest: "application/json",
	webmanifest: "application/manifest+json",
	json: "application/json",
	jsonl: "application/x-jsonlines",
	ndjson: "application/x-jsonlines",
	jsonc: "application/json",
	xml: "application/xml",
	yaml: "text/yaml",
	yml: "text/yaml",
	toml: "text/x-toml",
	ini: "text/plain",
	conf: "text/plain",
	config: "text/plain",
	cfg: "text/plain",
	env: "text/plain",
	properties: "text/plain",
	editorconfig: "text/plain",
	map: "application/json",
	tf: "text/plain",
	tfvars: "text/plain",
	tfstate: "application/json",
	j2: "text/plain",
	jinja: "text/plain",
	jinja2: "text/plain",
	cmake: "text/plain",
	gradle: "text/plain",
	pom: "application/xml",
	sln: "text/plain",
	slnx: "text/plain",
	csproj: "application/xml",
	vcxproj: "application/xml",
	fsproj: "application/xml",
	lock: "text/plain",
	diff: "text/plain",
	patch: "text/plain",
	log: "text/plain",
	dockerfile: "text/plain",
	makefile: "text/plain",
	gitignore: "text/plain",
	npmrc: "text/plain",

	// 4. Images and Graphics
	jpg: "image/jpeg",
	jpeg: "image/jpeg",
	jpe: "image/jpeg",
	png: "image/png",
	gif: "image/gif",
	webp: "image/webp",
	avif: "image/avif",
	heif: "image/heif",
	heic: "image/heic",
	bmp: "image/bmp",
	dib: "image/bmp",
	tif: "image/tiff",
	tiff: "image/tiff",
	svg: "image/svg+xml",
	ico: "image/x-icon",
	cur: "image/x-icon",
	psd: "image/vnd.adobe.photoshop",
	psb: "image/vnd.adobe.photoshop",
	ai: "application/postscript",
	indd: "application/x-indesign",
	idml: "application/vnd.adobe.indesign-idml-package",
	cdr: "application/octet-stream",
	xcf: "image/x-xcf",
	kra: "application/x-krita",
	afphoto: "application/octet-stream",
	afdesign: "application/octet-stream",
	raw: "image/x-raw",
	dng: "image/x-adobe-dng",
	cr2: "image/x-canon-cr2",
	cr3: "image/x-canon-cr3",
	nef: "image/x-nikon-nef",
	arw: "image/x-sony-arw",
	orf: "image/x-olympus-orf",
	rw2: "image/x-panasonic-rw2",
	raf: "image/x-fuji-raf",
	jp2: "image/jp2",
	j2k: "image/jp2",
	pbm: "image/x-portable-bitmap",
	pgm: "image/x-portable-graymap",
	ppm: "image/x-portable-pixmap",
	pnm: "image/x-portable-anymap",
	exr: "image/x-exr",
	hdr: "image/vnd.radiance",
	tga: "image/x-tga",
	dds: "image/x-dds",
	ktx: "image/ktx",
	ktx2: "image/ktx2",
	blend: "application/x-blender",
	ora: "image/openraster",
	wmf: "image/wmf",
	emf: "image/emf",
	jxl: "image/jxl",

	// 5. Audio Files
	mp3: "audio/mpeg",
	wav: "audio/wav",
	aac: "audio/aac",
	m4a: "audio/mp4",
	flac: "audio/flac",
	ogg: "audio/ogg",
	opus: "audio/opus",
	wma: "audio/x-ms-wma",
	aiff: "audio/aiff",
	aif: "audio/aiff",
	mid: "audio/midi",
	midi: "audio/midi",
	amr: "audio/amr",
	ra: "audio/x-realaudio",
	ram: "audio/x-pn-realaudio",
	au: "audio/basic",
	caf: "audio/x-caf",
	ape: "audio/x-ape",
	ac3: "audio/ac3",
	dts: "audio/vnd.dts",
	weba: "audio/webm",
	pcm: "audio/pcm",
	sf2: "application/octet-stream",
	sfz: "text/plain",
	aup3: "application/octet-stream",
	band: "application/octet-stream",
	als: "application/octet-stream",
	flp: "application/octet-stream",
	logicx: "application/octet-stream",
	ptx: "application/octet-stream",

	// 6. Video and Animation
	mp4: "video/mp4",
	m4v: "video/x-m4v",
	mpeg: "video/mpeg",
	mpg: "video/mpeg",
	mov: "video/quicktime",
	avi: "video/x-msvideo",
	mkv: "video/x-matroska",
	webm: "video/webm",
	wmv: "video/x-ms-wmv",
	flv: "video/x-flv",
	"3gp": "video/3gpp",
	"3g2": "video/3gpp2",
	m2ts: "video/mp2t",
	vob: "video/dvd",
	ogv: "video/ogg",
	qtl: "application/x-quicktimeplayer",
	lottie: "application/json",
	aep: "application/octet-stream",
	aepx: "application/xml",
	prproj: "application/xml",
	drp: "application/octet-stream",
	fcpxml: "application/xml",
	avp: "application/octet-stream",
	srt: "text/plain",
	vtt: "text/vtt",
	ass: "text/plain",
	ssa: "text/plain",
	ttml: "application/ttml+xml",
	dfxp: "application/ttaf+xml",
	m3u8: "application/x-mpegurl",
	mpd: "application/dash+xml",
	m4s: "video/iso.segment",

	// 7. Compressed Files and Archives
	zip: "application/zip",
	rar: "application/vnd.rar",
	"7z": "application/x-7z-compressed",
	tar: "application/x-tar",
	gz: "application/gzip",
	gzip: "application/gzip",
	tgz: "application/gzip",
	bz2: "application/x-bzip2",
	tbz2: "application/x-bzip2",
	xz: "application/x-xz",
	txz: "application/x-xz",
	zst: "application/zstd",
	cab: "application/vnd.ms-cab-compressed",
	iso: "application/x-iso9660-image",
	dmg: "application/x-apple-diskimage",
	img: "application/octet-stream",
	wim: "application/octet-stream",
	msi: "application/x-msi",
	ear: "application/java-archive",
	apk: "application/vnd.android.package-archive",
	aab: "application/octet-stream",
	deb: "application/vnd.debian.binary-package",
	rpm: "application/x-rpm",
	appimage: "application/octet-stream",
	pkg: "application/octet-stream",
	app: "application/octet-stream",
	sfx: "application/octet-stream",
	lzh: "application/x-lzh-compressed",
	lha: "application/x-lzh-compressed",
	z: "application/x-compress",
	par2: "application/octet-stream",

	// 8. Database and Data Storage Files
	db: "application/octet-stream",
	sqlite: "application/vnd.sqlite3",
	sqlite3: "application/vnd.sqlite3",
	dump: "text/plain",
	backup: "application/octet-stream",
	mdb: "application/x-msaccess",
	accdb: "application/msaccess",
	bak: "application/octet-stream",
	trn: "application/octet-stream",
	dmp: "application/octet-stream",
	exp: "application/octet-stream",
	dat: "application/octet-stream",
	duckdb: "application/octet-stream",
	ldb: "application/octet-stream",
	sst: "application/octet-stream",
	parquet: "application/vnd.apache.parquet",
	orc: "application/octet-stream",
	avro: "application/avro",
	feather: "application/octet-stream",
	arrow: "application/vnd.apache.arrow.file",
	arrows: "application/vnd.apache.arrow.stream",
	h5: "application/x-hdf5",
	hdf5: "application/x-hdf5",
	nc: "application/x-netcdf",
	pkl: "application/octet-stream",
	pickle: "application/octet-stream",
	joblib: "application/octet-stream",
	pb: "application/octet-stream",
	protobuf: "application/octet-stream",
	msgpack: "application/msgpack",
	mpk: "application/msgpack",
	bson: "application/bson",
	cbor: "application/cbor",
	sas7bdat: "application/octet-stream",
	sas7bcat: "application/octet-stream",
	xpt: "application/octet-stream",
	sav: "application/octet-stream",
	zsav: "application/octet-stream",
	por: "application/octet-stream",
	dta: "application/octet-stream",
	wal: "application/octet-stream",
	ibd: "application/octet-stream",

	// 9. AI, Machine Learning and Deep Learning Files
	pt: "application/octet-stream",
	pth: "application/octet-stream",
	jit: "application/octet-stream",
	tflite: "application/octet-stream",
	keras: "application/octet-stream",
	onnx: "application/octet-stream",
	blob: "application/octet-stream",
	engine: "application/octet-stream",
	plan: "application/octet-stream",
	mlmodel: "application/octet-stream",
	mlpackage: "application/octet-stream",
	gguf: "application/octet-stream",
	ggml: "application/octet-stream",
	safetensors: "application/octet-stream",
	spm: "application/octet-stream",
	ckpt: "application/octet-stream",
	checkpoint: "application/octet-stream",
	npz: "application/octet-stream",
	npy: "application/octet-stream",
	ubj: "application/octet-stream",
	model: "application/octet-stream",
	cbm: "application/octet-stream",
	dvc: "text/plain",
	faiss: "application/octet-stream",
	index: "application/octet-stream",
	ann: "application/octet-stream",
	lance: "application/octet-stream",
	onnx_data: "application/octet-stream",

	// 10. Executable, Installer and Binary Files
	exe: "application/vnd.microsoft.portable-executable",
	dll: "application/vnd.microsoft.portable-executable",
	sys: "application/octet-stream",
	cpl: "application/octet-stream",
	scr: "application/octet-stream",
	msp: "application/octet-stream",
	msix: "application/msix",
	appx: "application/appx",
	appxbundle: "application/appxbundle",
	so: "application/x-sharedlib",
	dylib: "application/octet-stream",
	kext: "application/octet-stream",
	dex: "application/octet-stream",
	odex: "application/octet-stream",
	vdex: "application/octet-stream",
	elf: "application/x-elf",
	bin: "application/octet-stream",
	hex: "text/plain",
	fw: "application/octet-stream",
	obj: "application/octet-stream",
	o: "application/x-object",
	pdb: "application/octet-stream",
	dsym: "application/octet-stream",
	core: "application/octet-stream",

	// 11. Security, Certificates and Cryptographic Files
	pem: "application/x-pem-file",
	crt: "application/x-x509-ca-cert",
	cer: "application/x-x509-ca-cert",
	cert: "application/x-x509-ca-cert",
	csr: "application/pkcs10",
	der: "application/x-x509-ca-cert",
	p7b: "application/x-pkcs7-certificates",
	p7c: "application/x-pkcs7-mime",
	p12: "application/x-pkcs12",
	pfx: "application/x-pkcs12",
	asc: "text/plain",
	gpg: "application/pgp-encrypted",
	pgp: "application/pgp-encrypted",
	jks: "application/octet-stream",
	keystore: "application/octet-stream",
	pk8: "application/octet-stream",
	p7s: "application/pkcs7-signature",
	sig: "application/octet-stream",
	sha256: "text/plain",
	sha512: "text/plain",
	md5: "text/plain",
	sha1: "text/plain",
	hc: "application/octet-stream",
	spdx: "text/plain",

	// 12. Email, Messaging and Contact Files
	eml: "message/rfc822",
	msg: "application/vnd.ms-outlook",
	pst: "application/vnd.ms-outlook",
	ost: "application/vnd.ms-outlook",
	mbox: "application/mbox",
	emlx: "message/rfc822",
	vcf: "text/vcard",
	ics: "text/calendar",
	ical: "text/calendar",

	// 13. Fonts and Typography
	ttf: "font/ttf",
	otf: "font/otf",
	woff: "font/woff",
	woff2: "font/woff2",
	eot: "application/vnd.ms-fontobject",
	pfa: "font/type1",
	pfb: "font/type1",
	afm: "application/x-font-afm",
	pfm: "application/x-font-type1",
	ttc: "font/collection",
	otc: "font/collection",
	sfd: "text/plain",

	// 14. CAD, 3D Models, Engineering and Manufacturing
	dwg: "image/vnd.dwg",
	dxf: "image/vnd.dxf",
	step: "model/step",
	stp: "model/step",
	iges: "model/iges",
	igs: "model/iges",
	stl: "model/stl",
	gltf: "model/gltf+json",
	glb: "model/gltf-binary",
	fbx: "application/octet-stream",
	"3ds": "application/x-3ds",
	sldprt: "application/octet-stream",
	sldasm: "application/octet-stream",
	slddrw: "application/octet-stream",
	f3d: "application/octet-stream",
	f3z: "application/octet-stream",
	fcstd: "application/octet-stream",
	skp: "application/octet-stream",
	rvt: "application/octet-stream",
	rfa: "application/octet-stream",
	ifc: "application/octet-stream",
	x_t: "application/octet-stream",
	x_b: "application/octet-stream",
	scad: "text/plain",
	gcode: "text/plain",
	tap: "text/plain",
	kicad_sch: "text/plain",
	kicad_pcb: "text/plain",
	gbr: "text/plain",
	ger: "text/plain",
	gtl: "text/plain",
	gbl: "text/plain",
	brd: "text/plain",
	sch: "text/plain",
	pcbdoc: "application/octet-stream",
	schdoc: "application/octet-stream",
	vi: "application/octet-stream",
	lvproj: "application/octet-stream",
	slx: "application/octet-stream",
	mdl: "text/plain",
	cnc: "text/plain",
	stpnc: "text/plain",

	// 15. GIS, Maps and Geographic Data
	shp: "application/octet-stream",
	shx: "application/octet-stream",
	dbf: "application/octet-stream",
	prj: "text/plain",
	geojson: "application/geo+json",
	kml: "application/vnd.google-earth.kml+xml",
	kmz: "application/vnd.google-earth.kmz",
	gpkg: "application/geopackage+sqlite3",
	gpx: "application/gpx+xml",
	tab: "text/plain",
	mif: "text/plain",
	fgb: "application/octet-stream",
	topojson: "application/json",
	las: "application/octet-stream",
	laz: "application/octet-stream",
	grib: "application/octet-stream",
	grb: "application/octet-stream",
	grb2: "application/octet-stream",
	mbtiles: "application/vnd.sqlite3",
	pmtiles: "application/octet-stream",
	osm: "application/xml",
	pbf: "application/octet-stream",
	wkt: "text/plain",

	// 16. Operating System and System Files
	lnk: "application/x-ms-shortcut",
	reg: "text/plain",
	evtx: "application/octet-stream",
	pf: "application/octet-stream",
	service: "text/plain",
	socket: "text/plain",
	timer: "text/plain",
	cron: "text/plain",
	plist: "application/xml",
	crash: "text/plain",
	ips: "text/plain",
	swp: "application/octet-stream",
	swo: "application/octet-stream",
	vmdk: "application/octet-stream",
	vdi: "application/octet-stream",
	vhd: "application/octet-stream",
	vhdx: "application/octet-stream",
	qcow2: "application/octet-stream",
	vmx: "text/plain",
	vbox: "application/xml",
	ovf: "application/ovf",
	ova: "application/ovf",
	rom: "application/octet-stream",
	cap: "application/octet-stream",
	fd: "application/octet-stream",

	// 17. Scientific, Mathematical and Research Files
	bib: "text/plain",
	ris: "text/plain",
	enl: "application/octet-stream",
	enlx: "application/octet-stream",
	rdf: "application/rdf+xml",
	nb: "application/mathematica",
	wl: "text/plain",
	fits: "application/fits",
	fit: "application/fits",
	fts: "application/fits",
	dcm: "application/dicom",
	nii: "application/octet-stream",
	fa: "text/plain",
	fasta: "text/plain",
	fna: "text/plain",
	fq: "text/plain",
	fastq: "text/plain",
	bam: "application/octet-stream",
	sam: "text/plain",
	cif: "text/plain",
	qmd: "text/markdown",
	zarr: "application/octet-stream",

	// 18. E-commerce, Payments and Business Data
	ofx: "application/x-ofx",
	qfx: "application/x-qfx",
	qif: "application/x-qif",
	qbb: "application/octet-stream",
	qbm: "application/octet-stream",
	qbw: "application/octet-stream",
	edi: "text/plain",
	x12: "text/plain",

	// 19. Gaming, Virtual Reality and Digital Assets
	unity: "application/octet-stream",
	prefab: "application/octet-stream",
	asset: "application/octet-stream",
	unitypackage: "application/octet-stream",
	uproject: "application/json",
	uasset: "application/octet-stream",
	umap: "application/octet-stream",
	godot: "text/plain",
	tscn: "text/plain",
	tres: "text/plain",
	mcworld: "application/octet-stream",
	mca: "application/octet-stream",
	save: "application/octet-stream",
	pak: "application/octet-stream",
	pck: "application/octet-stream",
	bundle: "application/octet-stream",
	wrl: "model/vrml",
	x3d: "model/x3d+xml",
	x3db: "model/x3d+fastinfoset",
	x3dv: "model/x3d-vrml",
	usd: "model/vnd.usd",
	usda: "model/vnd.usda",
	usdc: "model/vnd.usdc",
	usdz: "model/vnd.usdz+zip",
	bvh: "text/plain",
	"3mf": "application/vnd.ms-package.3dmanufacturing-3dmodel+xml",
	amf: "application/x-amf",
	shader: "text/plain",
	hlsl: "text/plain",
	glsl: "text/plain",

	// 20. Miscellaneous and Specialized Formats
	warc: "application/warc",
	wacz: "application/octet-stream",
	mhtml: "message/rfc822",
	mht: "message/rfc822",
	har: "application/json",
	po: "text/plain",
	mo: "application/x-gettext-translation",
	xliff: "application/x-xliff+xml",
	xlf: "application/x-xliff+xml",
	strings: "text/plain",
	stringsdict: "application/xml",
	arb: "application/json",
	rc: "text/plain",
	res: "application/octet-stream",
	storyboard: "application/xml",
	xib: "application/xml",
	asice: "application/vnd.etsi.asic-e+zip",
	asics: "application/vnd.etsi.asic-s+zip",
};

export function resolveFileMimeType(file: File): string {
	if (file.type && file.type !== "application/octet-stream") {
		return file.type;
	}
	const ext = file.name.split(".").pop()?.toLowerCase() || "";
	return EXTENSION_MIME[ext] || "application/octet-stream";
}

const ZIP_PACKAGED_DOCUMENT_EXTENSIONS = new Set([
	"docx",
	"docm",
	"dotx",
	"dotm",
	"xlsx",
	"xlsm",
	"xlsb",
	"xltx",
	"xltm",
	"pptx",
	"pptm",
	"ppsx",
	"ppsm",
	"potx",
	"potm",
	"odt",
	"ott",
	"ods",
	"ots",
	"odp",
	"otp",
	"odg",
	"epub",
	"numbers",
]);

/** Use file signatures when browser MIME and extension are missing or misleading. */
async function detectFileMimeType(file: File): Promise<string> {
	const ext = file.name.split(".").pop()?.toLowerCase() || "";
	let bytes: Uint8Array;
	try {
		bytes = new Uint8Array(await file.slice(0, 512).arrayBuffer());
	} catch {
		return resolveFileMimeType(file);
	}
	const ascii = (start: number, length: number) => String.fromCharCode(...bytes.subarray(start, start + length));
	const startsWith = (...signature: number[]) => signature.every((byte, index) => bytes[index] === byte);
	let detected: string | undefined;

	if (ascii(0, 5) === "%PDF-") detected = "application/pdf";
	else if (startsWith(0xff, 0xfe) || startsWith(0xfe, 0xff) || startsWith(0xef, 0xbb, 0xbf)) {
		detected = "text/plain";
	} else if (startsWith(0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a)) detected = "image/png";
	else if (startsWith(0xff, 0xd8, 0xff)) detected = "image/jpeg";
	else if (["GIF87a", "GIF89a"].includes(ascii(0, 6))) detected = "image/gif";
	else if (ascii(0, 2) === "BM") detected = "image/bmp";
	else if (["II*\0", "MM\0*", "II+\0", "MM\0+"].includes(ascii(0, 4))) detected = "image/tiff";
	else if (ascii(0, 4) === "RIFF" && ascii(8, 4) === "WEBP") detected = "image/webp";
	else if (ascii(0, 4) === "RIFF" && ascii(8, 4) === "WAVE") detected = "audio/wav";
	else if (ascii(0, 4) === "RIFF" && ascii(8, 3) === "AVI") detected = "video/x-msvideo";
	else if (ascii(0, 4) === "fLaC") detected = "audio/flac";
	else if (ascii(0, 4) === "OggS") detected = ext === "ogv" ? "video/ogg" : "audio/ogg";
	else if (ascii(0, 3) === "ID3") detected = "audio/mpeg";
	else if (bytes.length >= 2 && bytes[0] === 0xff && (bytes[1] & 0xe0) === 0xe0) detected = "audio/mpeg";
	else if (startsWith(0x1a, 0x45, 0xdf, 0xa3)) {
		detected = ext === "webm" ? (file.type.startsWith("audio/") ? file.type : "video/webm") : "video/x-matroska";
	} else if (ascii(4, 4) === "ftyp") {
		const brand = ascii(8, 4).trim();
		if (/^(heic|heix|hevc|hevx|mif1|msf1)$/i.test(brand)) detected = "image/heic";
		else if (/^(avif|avis)$/i.test(brand)) detected = "image/avif";
		else if (ext === "weba" || /^(M4A|M4B|F4A)$/i.test(brand)) detected = "audio/mp4";
		else detected = brand === "qt" ? "video/quicktime" : "video/mp4";
	} else if (startsWith(0x50, 0x4b) && (bytes[2] === 0x03 || bytes[2] === 0x05 || bytes[2] === 0x07)) detected = "application/zip";
	else if (startsWith(0x1f, 0x8b)) detected = "application/gzip";
	else if (ascii(0, 3) === "BZh") detected = "application/x-bzip2";
	else if (startsWith(0xfd, 0x37, 0x7a, 0x58, 0x5a, 0x00)) detected = "application/x-xz";
	else if (startsWith(0x28, 0xb5, 0x2f, 0xfd)) detected = "application/zstd";
	else if (ascii(0, 6) === "7z\xbc\xaf'\x1c") detected = "application/x-7z-compressed";
	else if (ascii(0, 7) === "Rar!\x1a\x07\x00" || ascii(0, 7) === "Rar!\x1a\x07\x01") detected = "application/vnd.rar";
	else if (ascii(0, 4) === "MSCF") detected = "application/vnd.ms-cab-compressed";
	else if (ascii(0, 16) === "SQLite format 3\0") detected = "application/vnd.sqlite3";
	else if (ascii(257, 5) === "ustar") detected = "application/x-tar";
	else if (startsWith(0x4d, 0x5a)) detected = "application/vnd.microsoft.portable-executable";
	else if (startsWith(0x7f, 0x45, 0x4c, 0x46)) detected = "application/x-elf";
	else if (startsWith(0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1)) detected = "application/x-ole-storage";
	else if (bytes.length > 0 && !bytes.includes(0)) {
		try {
			const sample = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
			const printable = [...sample].filter((char) => char === "\n" || char === "\r" || char === "\t" || char >= " ").length;
			if (sample.length > 0 && printable / sample.length > 0.9) {
				const trimmed = sample.trimStart();
				detected = trimmed.startsWith("{") || trimmed.startsWith("[") ? "application/json" : "text/plain";
			}
		} catch {
			// Keep the extension or browser MIME when the sample is not valid UTF-8 text.
		}
	}

	if (detected === "application/zip" && ZIP_PACKAGED_DOCUMENT_EXTENSIONS.has(ext)) {
		return EXTENSION_MIME[ext] || resolveFileMimeType(file);
	}
	return detected || resolveFileMimeType(file);
}

export function isImageFile(file: File, mimeType?: string): boolean {
	const effectiveMime = mimeType || resolveFileMimeType(file);
	if (effectiveMime.startsWith("image/")) return true;
	const ext = file.name.split(".").pop()?.toLowerCase() || "";
	return [
		"png",
		"jpg",
		"jpeg",
		"jpe",
		"webp",
		"gif",
		"bmp",
		"dib",
		"svg",
		"tiff",
		"tif",
		"avif",
		"heic",
		"heif",
		"ico",
		"cur",
		"psd",
		"psb",
		"ai",
		"indd",
		"idml",
		"cdr",
		"xcf",
		"kra",
		"afphoto",
		"afdesign",
		"raw",
		"dng",
		"cr2",
		"cr3",
		"nef",
		"arw",
		"orf",
		"rw2",
		"raf",
		"jp2",
		"j2k",
		"pbm",
		"pgm",
		"ppm",
		"pnm",
		"exr",
		"hdr",
		"tga",
		"dds",
		"ktx",
		"ktx2",
		"blend",
		"ora",
		"wmf",
		"emf",
		"jxl",
	].includes(ext);
}

export function validatePromptAttachmentFile(file: File): string | null {
	if (file.size > MAX_PROMPT_ATTACHMENT_BYTES) {
		return `"${file.name}" is too large (max ${Math.round(MAX_PROMPT_ATTACHMENT_BYTES / (1024 * 1024))} MB)`;
	}
	return null;
}

export function fileToBase64(file: File, mimeType?: string): Promise<string> {
	return new Promise((resolve, reject) => {
		const reader = new FileReader();
		reader.onload = () => {
			const dataUrl = String(reader.result || "");
			if (!mimeType || !dataUrl.startsWith("data:")) {
				resolve(dataUrl);
				return;
			}
			const comma = dataUrl.indexOf(",");
			resolve(comma >= 0 ? `data:${mimeType};base64,${dataUrl.slice(comma + 1)}` : dataUrl);
		};
		reader.onerror = reject;
		reader.onabort = () => reject(new DOMException(`Reading "${file.name}" was aborted`, "AbortError"));
		reader.readAsDataURL(file);
	});
}

/** Formats every vision provider (OpenAI, Anthropic, Gemini, Bedrock) accepts as inline images. */
const MODEL_IMAGE_MIMES = new Set(["image/png", "image/jpeg", "image/webp", "image/gif"]);
/** Anthropic rejects inline images above 5 MB; stay under it with base64 overhead in mind. */
const MODEL_IMAGE_MAX_BYTES = 3.75 * 1024 * 1024;
const MODEL_IMAGE_MAX_DIMENSION = 2048;
/** Mistral OCR accepts images up to 20 MB. Avoid base64-encoding arbitrary or oversized files. */
const OCR_IMAGE_MAX_BYTES = 20 * 1024 * 1024;
const OCR_RAW_IMAGE_MIMES = new Set(["image/png", "image/jpeg", "image/gif", "image/bmp", "image/tiff", "image/webp", "image/avif"]);
const GATEWAY_TRANSCRIPTION_EXTENSIONS = new Set(["flac", "mp3", "mp4", "mpeg", "mpga", "m4a", "ogg", "wav", "webm"]);

function loadImageElement(file: File): Promise<HTMLImageElement> {
	return new Promise((resolve, reject) => {
		const url = URL.createObjectURL(file);
		const img = new Image();
		img.onload = () => {
			URL.revokeObjectURL(url);
			resolve(img);
		};
		img.onerror = () => {
			URL.revokeObjectURL(url);
			reject(new Error(`Could not decode image "${file.name}"`));
		};
		img.src = url;
	});
}

/**
 * Returns a data URL a vision model will accept: supported format, at most
 * MODEL_IMAGE_MAX_DIMENSION px per side and under MODEL_IMAGE_MAX_BYTES.
 * Returns null when the browser cannot decode the image (e.g. HEIC outside Safari).
 */
export async function normalizeImageForModel(file: File, mimeType: string): Promise<string | null> {
	const supported = MODEL_IMAGE_MIMES.has(mimeType);
	try {
		const img = await loadImageElement(file);
		const width = img.naturalWidth;
		const height = img.naturalHeight;
		if (width <= 0 || height <= 0) return null;
		if (supported && file.size <= MODEL_IMAGE_MAX_BYTES && Math.max(width, height) <= MODEL_IMAGE_MAX_DIMENSION) {
			return await fileToBase64(file, mimeType);
		}
		let scale = Math.min(1, MODEL_IMAGE_MAX_DIMENSION / Math.max(width, height));
		const keepAlpha = mimeType === "image/png" || mimeType === "image/svg+xml" || mimeType === "image/x-icon" || mimeType === "image/webp";

		for (let attempt = 0; attempt < 5; attempt++) {
			const canvas = document.createElement("canvas");
			canvas.width = Math.max(1, Math.round(width * scale));
			canvas.height = Math.max(1, Math.round(height * scale));
			const g = canvas.getContext("2d");
			if (!g) return null;
			const asPng = keepAlpha && attempt === 0;
			if (!asPng) {
				g.fillStyle = "#ffffff";
				g.fillRect(0, 0, canvas.width, canvas.height);
			}
			g.drawImage(img, 0, 0, canvas.width, canvas.height);
			const dataUrl = asPng ? canvas.toDataURL("image/png") : canvas.toDataURL("image/jpeg", attempt <= 1 ? 0.9 : 0.8);
			const bytes = Math.ceil(((dataUrl.length - dataUrl.indexOf(",") - 1) * 3) / 4);
			if (bytes <= MODEL_IMAGE_MAX_BYTES) {
				if (!supported || scale < 1) {
					toast.message(`"${file.name}" converted for model compatibility`, {
						description: `${canvas.width}×${canvas.height} ${dataUrl.startsWith("data:image/png") ? "PNG" : "JPEG"}`,
					});
				}
				return dataUrl;
			}
			scale *= 0.75;
		}
		return null;
	} catch {
		return null;
	}
}

/** Validate the signature before sending a browser-undecodable image to cloud OCR. */
async function rawImageForOcr(file: File, mimeType: string): Promise<string | null> {
	if (file.size <= 0 || file.size > OCR_IMAGE_MAX_BYTES || !OCR_RAW_IMAGE_MIMES.has(mimeType)) return null;
	try {
		const bytes = new Uint8Array(await file.slice(0, 16).arrayBuffer());
		const ascii = (start: number, length: number) => String.fromCharCode(...bytes.subarray(start, start + length));
		const matches =
			(mimeType === "image/png" && [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a].every((b, i) => bytes[i] === b)) ||
			(mimeType === "image/jpeg" && bytes[0] === 0xff && bytes[1] === 0xd8 && bytes[2] === 0xff) ||
			(mimeType === "image/gif" && ["GIF87a", "GIF89a"].includes(ascii(0, 6))) ||
			(mimeType === "image/bmp" && ascii(0, 2) === "BM") ||
			(mimeType === "image/tiff" && ["II*\0", "MM\0*", "II+\0", "MM\0+"].includes(ascii(0, 4))) ||
			(mimeType === "image/webp" && ascii(0, 4) === "RIFF" && ascii(8, 4) === "WEBP") ||
			(mimeType === "image/avif" && ascii(4, 4) === "ftyp" && /^(avif|avis)$/i.test(ascii(8, 4)));
		return matches ? await fileToBase64(file, mimeType) : null;
	} catch {
		return null;
	}
}

function isZipFile(file: File, mimeType: string): boolean {
	return isPromptArchive(file, mimeType);
}

function isPdfFile(file: File, mimeType: string): boolean {
	const lower = file.name.toLowerCase();
	return lower.endsWith(".pdf") || mimeType.includes("pdf");
}

/**
 * Convert one or more files into message attachments.
 * Zip archives are expanded and each supported inner file is imported.
 */
export type AttachmentImportOptions = {
	apiKeyId?: string;
	virtualKeyId?: string;
	archiveDepth?: number;
	silent?: boolean;
	skipAudioTranscription?: boolean;
};

/** Run tasks in parallel with a controlled concurrency limit to avoid freezing the browser on 100+ files */
async function runConcurrentPool<T, R>(items: T[], concurrency: number, fn: (item: T, index: number) => Promise<R>): Promise<R[]> {
	const results: R[] = new Array(items.length);
	let nextIndex = 0;
	const workers = Array.from({ length: Math.min(concurrency, items.length) }, async () => {
		while (nextIndex < items.length) {
			const index = nextIndex++;
			results[index] = await fn(items[index], index);
		}
	});
	await Promise.all(workers);
	return results;
}

export async function filesToAttachments(files: FileList | File[], opts?: AttachmentImportOptions): Promise<MessageContent[]> {
	const filesList = Array.from(files);
	if (filesList.length === 0) return [];

	const isBulk = filesList.length > 4;
	const bulkToastId = "bulk-import-progress";
	const effectiveOpts: AttachmentImportOptions = {
		...opts,
		silent: isBulk ? true : opts?.silent,
	};

	if (isBulk) {
		toast.loading(`Importing ${filesList.length} files (0/${filesList.length})...`, { id: bulkToastId });
	}

	let completedCount = 0;
	const failedFiles: { name: string; error: string }[] = [];

	const largestFileBytes = filesList.reduce((largest, file) => Math.max(largest, file.size), 0);
	const importConcurrency = largestFileBytes >= 64 * 1024 * 1024 ? 1 : largestFileBytes >= 16 * 1024 * 1024 ? 2 : 4;
	const perFileResults = await runConcurrentPool(filesList, importConcurrency, async (file) => {
		const error = validatePromptAttachmentFile(file);
		if (error) {
			if (!effectiveOpts.silent) toast.error(error);
			failedFiles.push({ name: file.name, error });
			completedCount++;
			if (isBulk && (completedCount % 5 === 0 || completedCount === filesList.length)) {
				toast.loading(`Importing ${filesList.length} files (${completedCount}/${filesList.length})...`, { id: bulkToastId });
			}
			return [];
		}

		try {
			const mimeType = await detectFileMimeType(file);
			let res: MessageContent[] = [];
			if (isZipFile(file, mimeType)) {
				res = await attachmentsFromZip(file, effectiveOpts, mimeType);
			} else {
				res = await expandFileAttachments(file, effectiveOpts, mimeType);
			}
			if (res.length === 0) {
				failedFiles.push({
					name: file.name,
					error: `No attachments were created for "${file.name}"`,
				});
			} else if (
				res.some(
					(attachment) =>
						attachment.type === "text" &&
						attachment.text?.startsWith("Voice transcript") &&
						attachment.text.includes("[Transcription unavailable:"),
				)
			) {
				failedFiles.push({ name: file.name, error: `Voice audio could not be transcribed: ${file.name}` });
			}
			completedCount++;
			if (isBulk && (completedCount % 5 === 0 || completedCount === filesList.length)) {
				toast.loading(`Importing ${filesList.length} files (${completedCount}/${filesList.length})...`, { id: bulkToastId });
			}
			return res;
		} catch (err) {
			const message = err instanceof Error ? err.message : `Failed to import "${file.name}"`;
			if (!effectiveOpts.silent) toast.error(message);
			failedFiles.push({ name: file.name, error: message });
			completedCount++;
			if (isBulk && (completedCount % 5 === 0 || completedCount === filesList.length)) {
				toast.loading(`Importing ${filesList.length} files (${completedCount}/${filesList.length})...`, { id: bulkToastId });
			}
			return [];
		}
	});

	const attachments: MessageContent[] = perFileResults.flat();

	if (isBulk) {
		if (failedFiles.length === 0) {
			toast.success(`Successfully imported all ${filesList.length} files! (${attachments.length} attachments ready)`, {
				id: bulkToastId,
			});
		} else {
			toast.warning(`Imported ${attachments.length} attachments (${failedFiles.length} file(s) failed)`, {
				id: bulkToastId,
				description: failedFiles
					.slice(0, 3)
					.map((f) => f.name)
					.join(", "),
			});
		}
	}

	return attachments;
}

async function attachmentsFromZip(zipFile: File, opts?: AttachmentImportOptions, detectedMimeType?: string): Promise<MessageContent[]> {
	const depth = opts?.archiveDepth ?? 0;
	if (depth > 3) {
		if (!opts?.silent) toast.error(`"${zipFile.name}" is nested too deeply to unpack`);
		return [];
	}
	if (!opts?.silent) toast.message(`Unpacking ${zipFile.name}…`);
	const archiveMimeType = detectedMimeType || (await detectFileMimeType(zipFile));
	const { files, skipped, errors } = await extractArchiveInnerFiles(zipFile, archiveMimeType);
	if (files.length === 0) {
		if (!opts?.silent) {
			toast.error(`No files found in "${zipFile.name}"`, {
				description: errors[0] || "The archive is empty or could not be opened.",
			});
		}
		return [];
	}
	if (!opts?.silent) toast.message(`Importing ${files.length} file(s) from ${zipFile.name}…`);
	const out: MessageContent[] = [];
	const nextOpts = { ...opts, archiveDepth: depth + 1 };
	for (const inner of files) {
		const error = validatePromptAttachmentFile(inner);
		if (error) {
			if (!opts?.silent) toast.error(error);
			continue;
		}
		try {
			const mimeType = await detectFileMimeType(inner);
			if (isPromptArchive(inner, mimeType)) {
				out.push(...(await attachmentsFromZip(inner, nextOpts, mimeType)));
			} else if (isImageFile(inner, mimeType)) {
				out.push(...(await imageFileAttachments(inner, mimeType, nextOpts)));
			} else {
				const attachment = await fileToAttachment(inner, nextOpts, mimeType);
				if (attachment) out.push(attachment);
			}
		} catch (err) {
			if (!opts?.silent) toast.error(err instanceof Error ? err.message : `Failed to import "${inner.name}"`);
		}
	}
	if (out.length > 0 && !opts?.silent) {
		const extra = skipped > 0 || errors.length > 0 ? ` (${skipped} skipped${errors.length ? `; ${errors[0]}` : ""})` : "";
		toast.success(`Imported ${out.length} file(s) from ${zipFile.name}${extra}`);
	}
	return out;
}

function textAttachmentFromExtract(fileName: string, extracted: string): MessageContent {
	return {
		type: "text",
		text: `Attached file: ${fileName}\n\n--- extracted content ---\n${extracted.trim()}`,
	};
}

async function tryPdfOcr(file: File, opts?: AttachmentImportOptions): Promise<{ text: string; model: string } | null> {
	if (!opts?.silent) toast.message(`Running OCR on scanned PDF ${file.name}…`);
	try {
		const dataUrl = await fileToBase64(file, "application/pdf");
		const ocr = await ocrPdfFile(dataUrl, { apiKeyId: opts?.apiKeyId, virtualKeyId: opts?.virtualKeyId });
		if (ocr?.text) {
			if (!opts?.silent && ocr.text.includes("[Truncated:")) {
				toast.warning(`OCR read only part of ${file.name}`, {
					description:
						"The OCR page range is marked in the attachment. Text-layer content may cover other pages; scanned-only text outside that range may be missing.",
				});
			} else if (!opts?.silent) {
				toast.success(`OCR extracted text from ${file.name} (${ocr.model})`);
			}
			return ocr;
		}
	} catch (error) {
		console.warn(`PDF OCR failed for "${file.name}":`, error);
	}
	return null;
}

function pdfTextLayerMissesPages(text: string): boolean {
	if (!text || text.includes("[Truncated:")) return false;
	const totalPages = Number(text.match(/\[Page\s+\d+\s+of\s+(\d+)\]/i)?.[1] || 0);
	if (totalPages <= 1) return false;
	const extractedPages = new Set(Array.from(text.matchAll(/\[Page\s+(\d+)\s+of\s+\d+\]/gi), (match) => Number(match[1])));
	return extractedPages.size < totalPages;
}

function isVideoFile(file: File, mimeType: string): boolean {
	if (mimeType.startsWith("video/")) return true;
	return /\.(mp4|mov|mkv|avi|m4v|wmv|mpeg|mpg|webm)$/i.test(file.name) && !mimeType.startsWith("audio/");
}

/** Image import keeps the picture for vision models and always extracts readable text (OCR / SVG). */
async function imageFileAttachments(file: File, mimeType: string, opts?: AttachmentImportOptions): Promise<MessageContent[]> {
	const out: MessageContent[] = [];
	const dataUrl = await normalizeImageForModel(file, mimeType);
	const ocrSource = dataUrl || (await rawImageForOcr(file, mimeType));
	let extracted = "";
	if (ocrSource) {
		if (!opts?.silent) toast.message(`Extracting text from image ${file.name}…`);
		try {
			const ocr = await ocrImageFile(ocrSource, { apiKeyId: opts?.apiKeyId, virtualKeyId: opts?.virtualKeyId });
			if (ocr?.text?.trim()) extracted = ocr.text.trim();
		} catch (error) {
			console.warn(`OCR failed for image "${file.name}":`, error);
		}
	}
	if (!extracted) {
		const fallback = await extractPromptFileText(file, mimeType);
		if (fallback?.trim() && !fallback.startsWith("[Binary File:") && !fallback.includes("[No readable text content found")) {
			extracted = fallback.trim();
		}
	}
	if (dataUrl) {
		out.push({
			type: "image_url",
			image_url: {
				url: dataUrl,
				detail: "auto",
				filename: file.name,
				extracted_text: extracted || undefined,
			},
		});
	}
	if (extracted) {
		out.push(ocrTextAttachment(file.name, extracted));
	}
	if (out.length === 0) {
		out.push(
			textAttachmentFromExtract(
				file.name,
				`[Image: ${file.name} (${mimeType || "image"}, ${(file.size / 1024).toFixed(1)} KB) — image pixels could not be decoded and no readable text was extracted]`,
			),
		);
	}
	if (!opts?.silent) {
		if (dataUrl && extracted) {
			toast.success(`Image and extracted text attached: ${file.name}`);
		} else if (extracted) {
			toast.success(`Extracted text from image ${file.name}`);
		} else if (dataUrl) {
			toast.success(`Image attached: ${file.name}`);
		} else {
			toast.error(`Could not read image "${file.name}"`, {
				description:
					"The image could not be decoded and OCR found no readable text, so image questions cannot be answered from this upload.",
			});
		}
	}
	return out;
}

/** Capture a representative frame from a video file in the browser for visual analysis and screen OCR */
async function extractVideoKeyframe(file: File): Promise<string | null> {
	if (typeof window === "undefined" || typeof document === "undefined") return null;
	return new Promise((resolve) => {
		try {
			const video = document.createElement("video");
			video.preload = "metadata";
			video.muted = true;
			video.playsInline = true;
			const objUrl = URL.createObjectURL(file);
			video.src = objUrl;

			let finished = false;
			const cleanup = (res: string | null) => {
				if (finished) return;
				finished = true;
				try {
					URL.revokeObjectURL(objUrl);
				} catch {}
				resolve(res);
			};

			video.onloadedmetadata = () => {
				const duration = video.duration || 1;
				video.currentTime = Math.min(1.0, duration * 0.3);
			};

			video.onseeked = () => {
				try {
					const canvas = document.createElement("canvas");
					canvas.width = Math.min(video.videoWidth || 1280, 1280);
					canvas.height = Math.min(video.videoHeight || 720, 720);
					const ctx = canvas.getContext("2d");
					if (ctx) {
						ctx.drawImage(video, 0, 0, canvas.width, canvas.height);
						const dataUrl = canvas.toDataURL("image/jpeg", 0.85);
						cleanup(dataUrl);
						return;
					}
				} catch {
					/* ignore */
				}
				cleanup(null);
			};

			video.onerror = () => cleanup(null);
			setTimeout(() => cleanup(null), 3500);
		} catch {
			resolve(null);
		}
	});
}

/** Video attachment creation with instant local preview URL, Whisper speech-to-text, and visual Screen OCR fallback */
async function videoFileAttachments(file: File, mimeType: string, opts?: AttachmentImportOptions): Promise<MessageContent[]> {
	let videoUrl = "";
	try {
		videoUrl = URL.createObjectURL(file);
	} catch (e) {
		console.warn("Could not create object URL for video:", e);
	}

	let transcriptText = "";

	const normalized = await normalizeAudioToWavFile(file);
	if (normalized) {
		toast.message(`Transcribing speech in video ${file.name}…`);
		try {
			const transcript = await transcribeAudioFile(normalized, { apiKeyId: opts?.apiKeyId, virtualKeyId: opts?.virtualKeyId });
			if (transcript && transcript.text?.trim()) {
				transcriptText = transcript.text.trim();
				toast.success(`Video speech transcribed (${transcript.model})`);
			}
		} catch (err) {
			console.warn("Video speech transcription skipped:", err);
		}
	}

	let screenOcrText = "";
	let keyframeDataUrl: string | null = null;

	// If no speech was found (e.g. silent screen recording), extract a keyframe from the video and run OCR
	if (!transcriptText) {
		try {
			keyframeDataUrl = await extractVideoKeyframe(file);
			if (keyframeDataUrl) {
				const ocr = await ocrImageFile(keyframeDataUrl, { apiKeyId: opts?.apiKeyId, virtualKeyId: opts?.virtualKeyId });
				if (ocr?.text?.trim()) {
					screenOcrText = ocr.text.trim();
					toast.success(`Extracted on-screen text from video (${file.name})`);
				}
			}
		} catch (err) {
			console.warn("Keyframe OCR failed:", err);
		}
	}

	if (!transcriptText && !screenOcrText) {
		toast.success(`Video attached: ${file.name}`);
	}

	const sizeMb = (file.size / (1024 * 1024)).toFixed(1);
	const effectiveExtracted = transcriptText || screenOcrText;

	const textContent = transcriptText
		? `Attached video: ${file.name}\n\n--- Video Speech Transcript ---\n${transcriptText}`
		: screenOcrText
			? `Attached video: ${file.name}\n\n--- Video Screen OCR Content ---\n${screenOcrText}`
			: `Attached video: ${file.name} (${sizeMb} MB | ${mimeType || "video"})\n\n[Visual video media attached for playback preview]`;

	const out: MessageContent[] = [];

	out.push({
		type: "video_url",
		video_url: {
			url: videoUrl,
			filename: file.name,
			mimeType: mimeType || "video/mp4",
			extracted_text: effectiveExtracted || undefined,
		},
		text: textContent,
	});

	if (transcriptText) {
		out.push({
			type: "text",
			text: `Attached video: ${file.name}\n\n--- Video Speech Transcript ---\n${transcriptText}`,
		});
	} else if (screenOcrText) {
		out.push(ocrTextAttachment(file.name + " (Screen Recording Content)", screenOcrText));
	}

	if (keyframeDataUrl) {
		out.push({
			type: "image_url",
			image_url: {
				url: keyframeDataUrl,
				detail: "auto",
				filename: `${file.name} (frame preview)`,
				extracted_text: screenOcrText || undefined,
			},
		});
	}

	return out;
}

/** One picked file can become several message parts (image + extracted text, zip members, etc.). */
async function expandFileAttachments(file: File, opts?: AttachmentImportOptions, detectedMimeType?: string): Promise<MessageContent[]> {
	const mimeType = detectedMimeType || (await detectFileMimeType(file));
	if (isZipFile(file, mimeType)) {
		return attachmentsFromZip(file, opts, mimeType);
	}
	if (isImageFile(file, mimeType)) {
		return imageFileAttachments(file, mimeType, opts);
	}
	if (isVideoFile(file, mimeType)) {
		return videoFileAttachments(file, mimeType, opts);
	}
	const one = await fileToAttachment(file, opts);
	return one ? [one] : [];
}

export async function fileToAttachment(
	file: File,
	opts?: AttachmentImportOptions,
	detectedMimeType?: string,
): Promise<MessageContent | null> {
	const mimeType = detectedMimeType || (await detectFileMimeType(file));

	if (isZipFile(file, mimeType)) {
		// Prefer filesToAttachments (keeps each inner file separate). Single-return path combines text.
		const many = await attachmentsFromZip(file, opts, mimeType);
		if (many.length === 0) return null;
		if (many.length === 1) return many[0];
		const combined = many
			.map((a) => {
				if (a.type === "text" && a.text) return a.text;
				if (a.type === "image_url") return "[Image attachment]";
				if (a.type === "video_url") return a.video_url?.filename ? `[Video: ${a.video_url.filename}]` : "[Video attachment]";
				if (a.type === "input_audio") return "[Audio attachment]";
				if (a.type === "file") return a.file?.filename ? `[File: ${a.file.filename}]` : "[File]";
				return "";
			})
			.filter(Boolean)
			.join("\n\n---\n\n");
		return {
			type: "text",
			text: `Attached zip: ${file.name}\n\n${combined}`,
		};
	}

	if (isImageFile(file, mimeType)) {
		const parts = await imageFileAttachments(file, mimeType, opts);
		return parts[0] ?? null;
	}

	if (isVideoFile(file, mimeType)) {
		const parts = await videoFileAttachments(file, mimeType, opts);
		return parts[0] ?? null;
	}

	const fileExt = file.name.split(".").pop()?.toLowerCase() || "";
	const audioLike =
		mimeType.startsWith("audio/") ||
		["mp3", "wav", "m4a", "flac", "ogg", "opus", "aac", "wma", "aiff", "aif", "amr", "weba", "mpga"].includes(fileExt);

	if (audioLike) {
		const detectedAudioMime = mimeType.startsWith("audio/");
		const originalFormat = audioFormatFromMimeOrName(mimeType, file.name);
		const canTranscribeOriginal = detectedAudioMime && GATEWAY_TRANSCRIPTION_EXTENSIONS.has(fileExt);
		const normalizationAttempted = !canTranscribeOriginal;
		const normalized = normalizationAttempted ? await normalizeAudioToWavFile(file) : null;
		const transcriptionFile = normalized || (canTranscribeOriginal ? file : null);
		let transcript: Awaited<ReturnType<typeof transcribeAudioFile>> = null;
		if (!opts?.skipAudioTranscription) {
			if (!opts?.silent) toast.message(`Transcribing voice ${file.name}…`);
			if (!transcriptionFile) {
				transcript = {
					text: "",
					model: "",
					error:
						file.size > MAX_AUDIO_TRANSCRIPTION_BYTES
							? "Audio is larger than the 25 MB transcription limit."
							: "This audio format is unsupported and could not be converted by the browser.",
				};
			} else if (transcriptionFile.size > MAX_AUDIO_TRANSCRIPTION_BYTES) {
				transcript = { text: "", model: "", error: "Converted audio is larger than the 25 MB transcription limit." };
			} else {
				transcript = await transcribeAudioFile(transcriptionFile, {
					apiKeyId: opts?.apiKeyId,
					virtualKeyId: opts?.virtualKeyId,
				});
			}
			if (transcript?.text) {
				if (!opts?.silent) toast.success(`Voice transcribed (${transcript.model})`);
				return voiceTranscriptAttachment(file.name, transcript.text);
			}
		}

		let rawAudioFile = normalized || (detectedAudioMime && (originalFormat === "mp3" || originalFormat === "wav") ? file : null);
		if (!rawAudioFile && !normalizationAttempted && file.size <= MAX_AUDIO_TRANSCRIPTION_BYTES) {
			rawAudioFile = await normalizeAudioToWavFile(file);
		}
		const rawAudioFormat = rawAudioFile
			? audioFormatFromMimeOrName(rawAudioFile === file ? mimeType : rawAudioFile.type, rawAudioFile.name)
			: null;
		if (rawAudioFile && rawAudioFile.size <= MAX_AUDIO_TRANSCRIPTION_BYTES && (rawAudioFormat === "mp3" || rawAudioFormat === "wav")) {
			try {
				const dataUrl = await fileToBase64(rawAudioFile, rawAudioFormat === "mp3" ? "audio/mpeg" : "audio/wav");
				const commaIdx = dataUrl.indexOf(",");
				const rawB64 = commaIdx >= 0 ? dataUrl.slice(commaIdx + 1) : dataUrl;
				if (!opts?.silent) {
					if (transcript && "error" in transcript) {
						toast.warning(`Voice transcription unavailable for ${file.name}`, { description: transcript.error });
					} else {
						toast.message(`Attached audio: ${file.name}`);
					}
				}
				return { type: "input_audio", input_audio: { data: rawB64, format: rawAudioFormat } };
			} catch (err) {
				const reason = err instanceof Error ? err.message : String(err);
				if (!opts?.silent) toast.error(`Could not attach voice audio: ${reason}`);
				return voiceTranscriptAttachment(file.name, `[Transcription unavailable: audio could not be attached (${reason}).]`);
			}
		}

		const reason =
			transcript && "error" in transcript
				? transcript.error
				: file.size > MAX_AUDIO_TRANSCRIPTION_BYTES
					? "Audio is larger than the 25 MB transcription limit."
					: rawAudioFile && rawAudioFile.size > MAX_AUDIO_TRANSCRIPTION_BYTES
						? "Converted audio is larger than the 25 MB transcription limit."
						: "No readable speech was returned.";
		if (!opts?.silent) {
			toast.error(`Voice was not transcribed: ${file.name}`, {
				description: `${reason} This audio format could not be converted to a model-compatible audio attachment.`,
			});
		}
		return voiceTranscriptAttachment(file.name, `[Transcription unavailable: ${reason}]`);
	}

	const extracted = await extractPromptFileText(file, mimeType);
	const extractedText = extracted ?? "";
	const pdf = isPdfFile(file, mimeType);

	// Scanned / image-only PDF: text layer empty or thin → OCR document_url
	if (pdf && (isThinExtractedText(extractedText) || pdfTextLayerMissesPages(extractedText))) {
		const ocr = await tryPdfOcr(file, opts);
		if (ocr) {
			const combinedText =
				ocr.model.startsWith("tesseract") && extractedText.trim()
					? `${ocr.text}\n\n[PDF text layer included to retain text from pages outside the local OCR range.]\n${extractedText}`
					: ocr.text;
			return ocrTextAttachment(file.name, combinedText);
		}
		if (extractedText.trim()) {
			if (!opts?.silent && pdfTextLayerMissesPages(extractedText)) {
				toast.warning(`Some pages in ${file.name} could not be read`, {
					description: "The extracted text layer misses pages; OCR is unavailable, so scanned-only content may be missing.",
				});
			} else if (!opts?.silent) {
				toast.message(`Using thin PDF text layer for ${file.name}`, {
					description: "OCR unavailable. Configure Mistral OCR for better scan results.",
				});
			}
			return textAttachmentFromExtract(file.name, extractedText);
		}
		if (!opts?.silent) {
			toast.error(`Could not extract text from scanned PDF "${file.name}"`, {
				description: "Configure a Mistral OCR key/model, or use a text-based PDF.",
			});
		}
		return null;
	}

	if (extractedText.trim()) {
		const isBinaryStub = extractedText.startsWith("[Binary File:") || extractedText.startsWith("[File:");
		const isSilent = Boolean(opts?.silent);
		if (isBinaryStub) {
			const lowerExtract = extractedText.toLowerCase();
			if (!isSilent && (lowerExtract.includes("no readable text content found") || lowerExtract.includes("extraction error:"))) {
				toast.error(`Could not read the contents of "${file.name}"`, {
					description: "Only file metadata is available, so answers about its contents may not be possible.",
				});
			} else if (!isSilent) {
				toast.warning(`Only part of "${file.name}" could be read`, {
					description: lowerExtract.includes("extracted strings & metadata")
						? "This format only yielded readable text fragments; the full file contents are unavailable."
						: "The extracted text is incomplete; ask only about the available portion.",
				});
			}
		} else {
			if (!isSilent && extractedText.includes("[Truncated:")) {
				toast.warning(`Part of ${file.name} was extracted`, {
					description: "The extracted text reached the processing limit; the assistant will be told that the file is incomplete.",
				});
			} else if (!isSilent) {
				toast.success(`Extracted text from ${file.name}`);
			}
		}
		return textAttachmentFromExtract(file.name, extractedText);
	}

	toast.error(`Could not process "${file.name}"`, {
		description: "The file appears to be empty or completely unreadable.",
	});
	return null;
}

export function getAttachmentDisplayName(attachment: MessageContent): string {
	if (attachment.type === "video_url" || attachment.video_url) return attachment.video_url?.filename || "Video";
	if (attachment.type === "image_url") return attachment.image_url?.filename || "";
	if (attachment.type === "input_audio") return attachment.input_audio?.format?.toUpperCase() || "";
	if (attachment.type === "text" && attachment.text?.startsWith("Attached video:")) {
		const firstLine = attachment.text.split("\n")[0] || "";
		return firstLine.replace(/^Attached video:\s*/i, "").trim() || "Video";
	}
	if (attachment.type === "text" && attachment.text?.startsWith("Voice transcript")) return "Voice transcript";
	if (attachment.type === "text" && attachment.text?.includes("--- OCR extracted content ---")) {
		const firstLine = attachment.text.split("\n")[0] || "";
		const parsed = firstLine.replace(/^Attached file:\s*/i, "").trim();
		return parsed ? `OCR: ${parsed}` : "";
	}
	if (attachment.type === "text" && attachment.text?.startsWith("Attached file:")) {
		const firstLine = attachment.text.split("\n")[0] || "";
		return firstLine.replace(/^Attached file:\s*/i, "").trim();
	}
	return attachment.file?.filename || "";
}

export function attachmentNeedsVision(attachments: MessageContent[]): boolean {
	return attachments.some((a) => a.type === "image_url");
}

export function attachmentNeedsVideo(attachments: MessageContent[]): boolean {
	return attachments.some((a) => a.type === "video_url" || !!a.video_url);
}

export function attachmentNeedsAudio(attachments: MessageContent[]): boolean {
	return attachments.some((a) => a.type === "input_audio");
}
