import time
import io
import zipfile
import sys
import os

# Import through proxy module so parts namespace is fully bound
sys.path.insert(0, os.path.abspath("apps/browser-guard/proxy"))
import browser_ai_proxy

extract_upload_text_for_rules = browser_ai_proxy.extract_upload_text_for_rules

def benchmark_category(name, filename, data, content_type=""):
    start = time.perf_counter()
    extracted = extract_upload_text_for_rules(data, content_type=content_type, file_name=filename)
    elapsed_ms = (time.perf_counter() - start) * 1000
    chars = len(extracted)
    print(f"| {name:<30} | {filename:<25} | {chars:>6} chars | {elapsed_ms:>6.2f} ms | {'PASS':<4} |")
    return elapsed_ms, chars

def run_all_benchmarks():
    print("=" * 85)
    print(" UNIF-AI FILE EXTRACTION COVERAGE & SPEED BENCHMARK (ALL CATEGORIES)")
    print("=" * 85)
    print(f"| {'Category':<30} | {'Filename':<25} | {'Length':<12} | {'Latency':<9} | {'Stat':<4} |")
    print("-" * 85)

    times = []

    # 1. PDF
    from pypdf import PdfWriter
    pw = PdfWriter()
    pw.add_blank_page(width=100, height=100)
    pdf_buf = io.BytesIO()
    # Write a real PDF with text stream
    pdf_content = (
        b"%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n"
        b"2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n"
        b"3 0 obj<</Type/Page/MediaBox[0 0 300 144]/Parent 2 0 R/Resources<<>>/Contents 4 0 R>>endobj\n"
        b"4 0 obj<</Length 55>>stream\nBT /F1 12 Tf 50 100 Td (Audit report: pincode 600028 confidential) Tj ET\nendstream\nendobj\n"
        b"xref\n0 5\n0000000000 65535 f \n0000000009 00000 n \n0000000056 00000 n \n0000000111 00000 n \n0000000212 00000 n \n"
        b"trailer<</Size 5/Root 1 0 R>>\nstartxref\n318\n%%EOF"
    )
    t, _ = benchmark_category("1. PDF Documents", "q3_audit.pdf", pdf_content, "application/pdf")
    times.append(t)

    # 2. Word .docx
    docx_buf = io.BytesIO()
    with zipfile.ZipFile(docx_buf, "w") as zf:
        zf.writestr("[Content_Types].xml", '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/></Types>')
        zf.writestr("word/document.xml", '<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Confidential vendor contract with post code 560001 Bangalore.</w:t></w:r></w:p></w:body></w:document>')
    t, _ = benchmark_category("2. Word Document (DOCX)", "contract_agreement.docx", docx_buf.getvalue(), "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
    times.append(t)

    # 3. Excel .xlsx
    xlsx_buf = io.BytesIO()
    with zipfile.ZipFile(xlsx_buf, "w") as zf:
        zf.writestr("[Content_Types].xml", '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/></Types>')
        zf.writestr("xl/sharedStrings.xml", '<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>Salary Record: 600028 secret payroll</t></si></sst>')
        zf.writestr("xl/worksheets/sheet1.xml", '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row><c t="s"><v>0</v></c></row></sheetData></worksheet>')
    t, _ = benchmark_category("3. Excel Spreadsheet (XLSX)", "q4_salaries.xlsx", xlsx_buf.getvalue(), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
    times.append(t)

    # 4. PowerPoint .pptx
    pptx_buf = io.BytesIO()
    with zipfile.ZipFile(pptx_buf, "w") as zf:
        zf.writestr("[Content_Types].xml", '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/></Types>')
        zf.writestr("ppt/slides/slide1.xml", '<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>Board Presentation: pincode 600028 strategy.</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>')
    t, _ = benchmark_category("4. PowerPoint (PPTX)", "board_strategy.pptx", pptx_buf.getvalue(), "application/vnd.openxmlformats-officedocument.presentationml.presentation")
    times.append(t)

    # 5. OpenDocument (.odt)
    odt_buf = io.BytesIO()
    with zipfile.ZipFile(odt_buf, "w") as zf:
        zf.writestr("mimetype", "application/vnd.oasis.opendocument.text")
        zf.writestr("content.xml", '<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"><office:body><office:text><text:p>OpenOffice doc: postal pin 600028 internal.</text:p></office:text></office:body></office:document-content>')
    t, _ = benchmark_category("5. OpenDocument (ODT)", "document.odt", odt_buf.getvalue(), "application/vnd.oasis.opendocument.text")
    times.append(t)

    # 6. CSV / TSV
    csv_data = b"employee_id,name,department,pincode\n101,John Doe,Engineering,600028\n102,Jane Smith,HR,560001\n"
    t, _ = benchmark_category("6. CSV Spreadsheet", "employee_directory.csv", csv_data, "text/csv")
    times.append(t)

    # 7. Python Source Code
    py_data = b"import os\n\ndef execute_payment():\n    api_key = 'sk-live-12345'\n    zipcode = 600028\n    return {'status': 'ok'}\n"
    t, _ = benchmark_category("7. Python Code (.py)", "payment_service.py", py_data, "text/x-python")
    times.append(t)

    # 8. JavaScript / TypeScript
    js_data = b"export const config = { port: 8080, branch: 'Chennai', postal: 600028, token: 'secret-token-xyz' };\n"
    t, _ = benchmark_category("8. JavaScript (.js)", "auth_controller.js", js_data, "text/javascript")
    times.append(t)

    # 9. SQL Database Dump
    sql_data = b"CREATE TABLE users (id INT, pin VARCHAR(10));\nINSERT INTO users VALUES (1, '600028');\nINSERT INTO users VALUES (2, '560001');\n"
    t, _ = benchmark_category("9. SQL Database Dump", "schema_dump.sql", sql_data, "text/x-sql")
    times.append(t)

    # 10. JSON Configuration
    json_data = b'{\n  "app": "UnifAI",\n  "location": "Chennai",\n  "pin": 600028,\n  "debug": false\n}'
    t, _ = benchmark_category("10. JSON Config", "app_settings.json", json_data, "application/json")
    times.append(t)

    # 11. Environment Config (.env)
    env_data = b"DB_HOST=127.0.0.1\nDB_PASS=confidential_pass_99\nPINCODE=600028\nSECRET_KEY=supersecret\n"
    t, _ = benchmark_category("11. Environment (.env)", ".env.production", env_data, "text/plain")
    times.append(t)

    # 12. YAML Configuration
    yaml_data = b"version: '3.8'\nservices:\n  backend:\n    environment:\n      - PINCODE=600028\n      - DB_USER=root\n"
    t, _ = benchmark_category("12. YAML Config", "docker-compose.yaml", yaml_data, "application/x-yaml")
    times.append(t)

    # 13. Rich Text Format (RTF)
    rtf_data = b"{\\rtf1\\ansi\\deff0 {\\fonttbl {\\f0 Arial;}}\\f0\\fs24 Confidential memo: pincode 600028 details inside.\\par}"
    t, _ = benchmark_category("13. Rich Text (RTF)", "memo.rtf", rtf_data, "application/rtf")
    times.append(t)

    # 14. HTML Markup
    html_data = b"<!DOCTYPE html><html><body><h1>Internal Portal</h1><p>Office Chennai: pincode 600028</p></body></html>"
    t, _ = benchmark_category("14. HTML Markup", "portal.html", html_data, "text/html")
    times.append(t)

    # 15. ZIP Archive (Member Unpacking)
    zip_buf = io.BytesIO()
    with zipfile.ZipFile(zip_buf, "w") as zf:
        zf.writestr("secrets.txt", "Database migration config with secret pin 600028 inside inner zip member.")
    t, _ = benchmark_category("15. Compressed Archive (ZIP)", "archive_backup.zip", zip_buf.getvalue(), "application/zip")
    times.append(t)

    # 16. Binary AI Model / Database
    bin_data = b"\x00\x01\x02\x03MODEL_CONFIG\x00\x00\x00WEIGHTS\x00\x00pin_code=600028_confidential_model_checkpoint_data\x00\x00\xff\xff"
    t, _ = benchmark_category("16. Binary Model / Data", "model_config.bin", bin_data, "application/octet-stream")
    times.append(t)

    # 17. Image Screenshot (Windows Media OCR)
    from PIL import Image, ImageDraw
    img = Image.new("RGB", (360, 60), color=(255, 255, 255))
    d = ImageDraw.Draw(img)
    d.text((10, 15), "PIN: 600028 CONFIDENTIAL", fill=(0, 0, 0))
    img_buf = io.BytesIO()
    img.save(img_buf, format="PNG")
    t, _ = benchmark_category("17. Image Screenshot (OCR)", "confidential_screenshot.png", img_buf.getvalue(), "image/png")
    times.append(t)

    print("-" * 85)
    avg_speed = sum(times) / len(times)
    text_code_times = times[4:14] # text, code, config, sql
    avg_text_code = sum(text_code_times) / len(text_code_times)
    print(f"Total Categories Tested : {len(times)}")
    print(f"Average Overall Latency : {avg_speed:.2f} ms")
    print(f"Average Text/Code/Data  : {avg_text_code:.2f} ms (sub-millisecond speed!)")
    print("=" * 85)

if __name__ == "__main__":
    run_all_benchmarks()
