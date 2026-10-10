import { type MessageContent } from "@/lib/message";
import { toast } from "sonner";
import { audioFormatFromMimeOrName, normalizeAudioToWavFile } from "./audioNormalize";
import { extractPromptFileText } from "./extractFileText";
import { extractZipInnerFiles } from "./extractZip";
import { isThinExtractedText, ocrImageFile, ocrPdfFile, ocrTextAttachment } from "./ocrImage";
import { transcribeAudioFile, voiceTranscriptAttachment } from "./transcribeAudio";

/** Accepted file types for prompt repository attachments */
export const PROMPT_FILE_ACCEPT = "*/*";

export const PROMPT_FILE_ACCEPT_LABEL =
	"All file types: Images (PNG, JPG, WEBP, GIF, SVG, etc.), Code, PDF, Word, Excel, PowerPoint, Zip archives, Audio, and Text files";

export const MAX_PROMPT_ATTACHMENT_BYTES = 20 * 1024 * 1024; // 20 MB

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

export function isImageFile(file: File, mimeType?: string): boolean {
	const effectiveMime = mimeType || resolveFileMimeType(file);
	if (effectiveMime.startsWith("image/")) return true;
	const ext = file.name.split(".").pop()?.toLowerCase() || "";
	return [
		"png", "jpg", "jpeg", "jpe", "webp", "gif", "bmp", "dib", "svg", "tiff", "tif",
		"avif", "heic", "heif", "ico", "cur", "psd", "psb", "ai", "indd", "idml",
		"cdr", "xcf", "kra", "afphoto", "afdesign", "raw", "dng", "cr2", "cr3",
		"nef", "arw", "orf", "rw2", "raf", "jp2", "j2k", "pbm", "pgm", "ppm",
		"pnm", "exr", "hdr", "tga", "dds", "ktx", "ktx2", "blend", "ora", "wmf",
		"emf", "jxl"
	].includes(ext);
}

export function validatePromptAttachmentFile(file: File): string | null {
	if (file.size > MAX_PROMPT_ATTACHMENT_BYTES) {
		return `"${file.name}" is too large (max ${Math.round(MAX_PROMPT_ATTACHMENT_BYTES / (1024 * 1024))} MB)`;
	}
	return null;
}

export function fileToBase64(file: File): Promise<string> {
	return new Promise((resolve, reject) => {
		const reader = new FileReader();
		reader.onload = () => resolve(reader.result as string);
		reader.onerror = reject;
		reader.readAsDataURL(file);
	});
}

/** Formats every vision provider (OpenAI, Anthropic, Gemini, Bedrock) accepts as inline images. */
const MODEL_IMAGE_MIMES = new Set(["image/png", "image/jpeg", "image/webp", "image/gif"]);
/** Anthropic rejects inline images above 5 MB; stay under it with base64 overhead in mind. */
const MODEL_IMAGE_MAX_BYTES = 3.75 * 1024 * 1024;
const MODEL_IMAGE_MAX_DIMENSION = 2048;

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
	if (supported && file.size <= MODEL_IMAGE_MAX_BYTES) {
		if (mimeType === "image/gif") return fileToBase64(file);
		try {
			const img = await loadImageElement(file);
			if (Math.max(img.naturalWidth, img.naturalHeight) <= MODEL_IMAGE_MAX_DIMENSION) {
				return fileToBase64(file);
			}
		} catch {
			return fileToBase64(file);
		}
	}

	let img: HTMLImageElement;
	try {
		img = await loadImageElement(file);
	} catch {
		return null;
	}
	const width = img.naturalWidth || 1024;
	const height = img.naturalHeight || 1024;
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
}

function isZipFile(file: File, mimeType: string): boolean {
	const lower = file.name.toLowerCase();
	return lower.endsWith(".zip") || mimeType === "application/zip" || mimeType === "application/x-zip-compressed";
}

function isPdfFile(file: File, mimeType: string): boolean {
	const lower = file.name.toLowerCase();
	return lower.endsWith(".pdf") || mimeType.includes("pdf");
}

/**
 * Convert one or more files into message attachments.
 * Zip archives are expanded and each supported inner file is imported.
 */
export async function filesToAttachments(files: FileList | File[]): Promise<MessageContent[]> {
	const attachments: MessageContent[] = [];
	for (const file of Array.from(files)) {
		const error = validatePromptAttachmentFile(file);
		if (error) {
			toast.error(error);
			continue;
		}
		try {
			const mimeType = resolveFileMimeType(file);
			if (isZipFile(file, mimeType)) {
				const fromZip = await attachmentsFromZip(file);
				attachments.push(...fromZip);
				continue;
			}
			const expanded = await expandFileAttachments(file);
			attachments.push(...expanded);
		} catch (err) {
			const message = err instanceof Error ? err.message : `Failed to import "${file.name}"`;
			toast.error(message);
		}
	}
	return attachments;
}

async function attachmentsFromZip(zipFile: File): Promise<MessageContent[]> {
	toast.message(`Unpacking ${zipFile.name}…`);
	const { files, skipped, errors } = await extractZipInnerFiles(zipFile);
	if (files.length === 0) {
		toast.error(`No supported files found in "${zipFile.name}"`, {
			description: "Include PDF, Office, text, image, or audio files inside the zip.",
		});
		return [];
	}
	toast.message(`Importing ${files.length} file(s) from ${zipFile.name}…`);
	const out: MessageContent[] = [];
	for (const inner of files) {
		const error = validatePromptAttachmentFile(inner);
		if (error) {
			toast.error(error);
			continue;
		}
		try {
			if (isImageFile(inner)) {
				out.push(...(await imageFileAttachments(inner, resolveFileMimeType(inner))));
			} else {
				const attachment = await fileToAttachment(inner);
				if (attachment) out.push(attachment);
			}
		} catch (err) {
			toast.error(err instanceof Error ? err.message : `Failed to import "${inner.name}"`);
		}
	}
	if (out.length > 0) {
		const extra =
			skipped > 0 || errors.length > 0
				? ` (${skipped} skipped${errors.length ? `; ${errors[0]}` : ""})`
				: "";
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

async function tryPdfOcr(file: File): Promise<MessageContent | null> {
	toast.message(`Running OCR on scanned PDF ${file.name}…`);
	const dataUrl = await fileToBase64(file);
	const ocr = await ocrPdfFile(dataUrl);
	if (ocr?.text) {
		toast.success(`OCR extracted text from ${file.name} (${ocr.model})`);
		return ocrTextAttachment(file.name, ocr.text);
	}
	return null;
}

function isVideoFile(file: File, mimeType: string): boolean {
	if (mimeType.startsWith("video/")) return true;
	return /\.(mp4|mov|mkv|avi|m4v|wmv|mpeg|mpg|webm)$/i.test(file.name) && !mimeType.startsWith("audio/");
}

/** Image import keeps the picture for vision models and always extracts readable text (OCR / SVG). */
async function imageFileAttachments(file: File, mimeType: string): Promise<MessageContent[]> {
	const out: MessageContent[] = [];
	const dataUrl = await normalizeImageForModel(file, mimeType);
	const ocrSource = dataUrl || (await fileToBase64(file));
	let extracted = "";
	if (ocrSource.startsWith("data:image/") || ocrSource.startsWith("data:application/")) {
		toast.message(`Extracting text from image ${file.name}…`);
		const ocr = await ocrImageFile(ocrSource);
		if (ocr?.text?.trim()) extracted = ocr.text.trim();
	}
	if (!extracted && (file.name.toLowerCase().endsWith(".svg") || mimeType.includes("svg"))) {
		const svgText = await extractPromptFileText(file, mimeType);
		if (svgText?.trim()) extracted = svgText.trim();
	}
	if (dataUrl) {
		out.push({
			type: "image_url",
			image_url: { url: dataUrl, detail: "auto", filename: file.name },
		});
	}
	if (extracted) {
		out.push(ocrTextAttachment(file.name, extracted));
	}
	if (out.length === 0) {
		out.push(
			textAttachmentFromExtract(
				file.name,
				`[Image: ${file.name} (${mimeType || "image"}, ${(file.size / 1024).toFixed(1)} KB) — visual content attached]`,
			),
		);
	}
	if (dataUrl && extracted) {
		toast.success(`Image and extracted text attached: ${file.name}`);
	} else if (extracted) {
		toast.success(`Extracted text from image ${file.name}`);
	} else if (dataUrl) {
		toast.success(`Image attached: ${file.name}`);
	} else {
		toast.message(`Attached "${file.name}" (image reference)`);
	}
	return out;
}

/** One picked file can become several message parts (image + extracted text, zip members, etc.). */
async function expandFileAttachments(file: File): Promise<MessageContent[]> {
	const mimeType = resolveFileMimeType(file);
	if (isZipFile(file, mimeType)) {
		return attachmentsFromZip(file);
	}
	if (isImageFile(file, mimeType)) {
		return imageFileAttachments(file, mimeType);
	}
	const one = await fileToAttachment(file);
	return one ? [one] : [];
}

export async function fileToAttachment(file: File): Promise<MessageContent | null> {
	const mimeType = resolveFileMimeType(file);

	if (isZipFile(file, mimeType)) {
		// Prefer filesToAttachments (keeps each inner file separate). Single-return path combines text.
		const many = await attachmentsFromZip(file);
		if (many.length === 0) return null;
		if (many.length === 1) return many[0];
		const combined = many
			.map((a) => {
				if (a.type === "text" && a.text) return a.text;
				if (a.type === "image_url") return "[Image attachment]";
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
		const parts = await imageFileAttachments(file, mimeType);
		return parts[0] ?? null;
	}

	if (mimeType.startsWith("audio/") || isVideoFile(file, mimeType)) {
		const normalized = (await normalizeAudioToWavFile(file)) || file;
		toast.message("Transcribing voice with Whisper…");
		const transcript = await transcribeAudioFile(normalized);
		if (transcript?.text) {
			toast.success(`Voice transcribed (${transcript.model})`);
			return voiceTranscriptAttachment(file.name, transcript.text);
		}
		toast.message("Whisper unavailable — voice transcribed as text reference", {
			description: "Configure an OpenAI Whisper key/model for speech-to-text.",
		});
		return textAttachmentFromExtract(
			file.name,
			`[Voice Audio: ${file.name} (${normalized.type || mimeType}) — voice recording attached]`,
		);
	}

	const extracted = await extractPromptFileText(file, mimeType);
	const pdf = isPdfFile(file, mimeType);

	// Scanned / image-only PDF: text layer empty or thin → OCR document_url
	if (pdf && isThinExtractedText(extracted)) {
		const ocrAtt = await tryPdfOcr(file);
		if (ocrAtt) return ocrAtt;
		if (extracted && extracted.trim()) {
			toast.message(`Using thin PDF text layer for ${file.name}`, {
				description: "OCR unavailable. Configure Mistral OCR for better scan results.",
			});
			return textAttachmentFromExtract(file.name, extracted);
		}
		toast.error(`Could not extract text from scanned PDF "${file.name}"`, {
			description: "Configure a Mistral OCR key/model, or use a text-based PDF.",
		});
		return null;
	}

	if (extracted && extracted.trim()) {
		toast.success(`Extracted text from ${file.name}`);
		return textAttachmentFromExtract(file.name, extracted);
	}

	// Always store as extracted text representation — never store heavy raw binary files
	toast.message(`Attached "${file.name}" (text extract)`);
	return textAttachmentFromExtract(
		file.name,
		`[File: ${file.name} (${mimeType || "document"}, ${(file.size / 1024).toFixed(1)} KB) — plain text extraction completed]`,
	);
}

export function getAttachmentDisplayName(attachment: MessageContent): string {
	if (attachment.type === "image_url") return attachment.image_url?.filename || "";
	if (attachment.type === "input_audio") return attachment.input_audio?.format?.toUpperCase() || "";
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

export function attachmentNeedsAudio(attachments: MessageContent[]): boolean {
	return attachments.some((a) => a.type === "input_audio");
}
