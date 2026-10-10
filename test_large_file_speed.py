import time
import io
import zipfile
import sys
import os

sys.path.insert(0, os.path.abspath("apps/browser-guard/proxy"))
import browser_ai_proxy
extract_upload_text_for_rules = browser_ai_proxy.extract_upload_text_for_rules

def run_stress_benchmarks():
    print("=" * 85)
    print(" STRESS / LARGE FILE EXTRACTION SPEED BENCHMARK")
    print("=" * 85)
    print(f"| {'File Description':<32} | {'Size':<12} | {'Extracted':<12} | {'Latency':<9} |")
    print("-" * 85)

    # 1. 5,000 lines of Python code (~150 KB)
    py_large = ("def process_item(i):\n    pin = 600028\n    return i * 2\n" * 2500).encode("utf-8")
    t0 = time.perf_counter()
    res = extract_upload_text_for_rules(py_large, file_name="large_backend.py")
    t_ms = (time.perf_counter() - t0) * 1000
    print(f"| {'5,000 lines Python code':<32} | {len(py_large)/1024:>8.1f} KB | {len(res):>6} chars | {t_ms:>6.2f} ms |")

    # 2. 10,000 row CSV file (~350 KB)
    csv_large = ("id,name,department,pin\n" + "101,John Doe,Engineering,600028\n" * 10000).encode("utf-8")
    t0 = time.perf_counter()
    res = extract_upload_text_for_rules(csv_large, file_name="large_database.csv", content_type="text/csv")
    t_ms = (time.perf_counter() - t0) * 1000
    print(f"| {'10,000 rows CSV database':<32} | {len(csv_large)/1024:>8.1f} KB | {len(res):>6} chars | {t_ms:>6.2f} ms |")

    # 3. 50-slide PowerPoint PPTX (~100 KB)
    pptx_buf = io.BytesIO()
    with zipfile.ZipFile(pptx_buf, "w") as zf:
        zf.writestr("[Content_Types].xml", '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/></Types>')
        for s in range(1, 51):
            zf.writestr(f"ppt/slides/slide{s}.xml", f'<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"><p:cSld><p:spTree><p:sp><p:txBody><a:p><a:r><a:t>Slide {s}: confidential strategy with pin 600028</a:t></a:r></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>')
    pptx_data = pptx_buf.getvalue()
    t0 = time.perf_counter()
    res = extract_upload_text_for_rules(pptx_data, file_name="company_all_hands.pptx")
    t_ms = (time.perf_counter() - t0) * 1000
    print(f"| {'50-slide PowerPoint presentation':<32} | {len(pptx_data)/1024:>8.1f} KB | {len(res):>6} chars | {t_ms:>6.2f} ms |")

    # 4. Multi-file ZIP with 10 inner files (~50 KB)
    zip_buf = io.BytesIO()
    with zipfile.ZipFile(zip_buf, "w") as zf:
        for i in range(10):
            zf.writestr(f"project_module_{i}.py", f"def run_task_{i}():\n    token = 'secret_pin_600028'\n    pass\n")
    zip_data = zip_buf.getvalue()
    t0 = time.perf_counter()
    res = extract_upload_text_for_rules(zip_data, file_name="project_repo.zip")
    t_ms = (time.perf_counter() - t0) * 1000
    print(f"| {'10-file ZIP archive bundle':<32} | {len(zip_data)/1024:>8.1f} KB | {len(res):>6} chars | {t_ms:>6.2f} ms |")

    # 5. Large JSON payload (~200 KB)
    json_large = ('[\n' + '  {"id": 1, "key": "val", "pin": 600028},\n' * 5000 + '  {"id": 999, "done": true}\n]').encode("utf-8")
    t0 = time.perf_counter()
    res = extract_upload_text_for_rules(json_large, file_name="export_dataset.json", content_type="application/json")
    t_ms = (time.perf_counter() - t0) * 1000
    print(f"| {'5,000 records JSON dataset':<32} | {len(json_large)/1024:>8.1f} KB | {len(res):>6} chars | {t_ms:>6.2f} ms |")

    print("=" * 85)

if __name__ == "__main__":
    run_stress_benchmarks()
