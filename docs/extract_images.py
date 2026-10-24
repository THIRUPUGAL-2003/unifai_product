import os
import pymupdf as fitz

out_dir = r"d:\unifai_project\docs\pdf_assets"
os.makedirs(out_dir, exist_ok=True)
doc = fitz.open(r"C:\Users\sakth\Downloads\infosec.pdf")

for page_idx, page in enumerate(doc):
    image_list = page.get_images(full=True)
    print(f"Page {page_idx+1}: {len(image_list)} images")
    for img_idx, img in enumerate(image_list):
        xref = img[0]
        base = doc.extract_image(xref)
        ext = base["ext"]
        fn = os.path.join(out_dir, f"p{page_idx+1}_img{img_idx+1}.{ext}")
        with open(fn, "wb") as f:
            f.write(base["image"])
        print(f"  Saved {fn} ({base['width']}x{base['height']})")
