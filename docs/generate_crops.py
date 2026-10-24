from pathlib import Path
from PIL import Image

ASSETS_DIR = Path(r"d:\unifai_project\docs\pdf_assets")

def create_crops():
    print("Generating dedicated crops...")
    
    # 1. Req 46: Shadow AI & Domain Locking
    im1 = Image.open(ASSETS_DIR / "extra_img_1.png")
    crop_46 = im1.crop((120, 100, 950, 420))
    crop_46.save(ASSETS_DIR / "crop_req46_shadow_ai.png")
    
    # 2. Req 47: Data Repositories & File Storage Monitor
    im3 = Image.open(ASSETS_DIR / "extra_img_3.png")
    crop_47 = im3.crop((120, 80, 950, 360))
    crop_47.save(ASSETS_DIR / "crop_req47_file_monitor.png")
    
    # 3. Req 50: Policy-Based Controls (Block vs Redact)
    im5 = Image.open(ASSETS_DIR / "extra_img_5.png")
    crop_50 = im5.crop((120, 110, 950, 420))
    crop_50.save(ASSETS_DIR / "crop_req50_policy_control.png")
    
    # 4. Req 53: Classification Mechanisms
    im_p1_2 = Image.open(ASSETS_DIR / "p1_img2.jpeg")
    crop_53 = im_p1_2.crop((200, 200, 1750, 750))
    crop_53.save(ASSETS_DIR / "crop_req53_classification.png")
    
    # 5. Req 56: Restricted Dataset Access Prevention
    crop_56 = im3.crop((120, 150, 950, 450))
    crop_56.save(ASSETS_DIR / "crop_req56_dataset_restriction.png")
    
    # 6. Req 59: Automated Data Exfiltration Alerting
    im_p4_1 = Image.open(ASSETS_DIR / "p4_img1.jpeg")
    crop_59 = im_p4_1.crop((200, 200, 1750, 750))
    crop_59.save(ASSETS_DIR / "crop_req59_exfiltration_alert.png")
    
    # 7. Req 60: Automated Scraping Mitigation
    im_p2_1 = Image.open(ASSETS_DIR / "p2_img1.jpeg")
    crop_60 = im_p2_1.crop((200, 250, 1750, 800))
    crop_60.save(ASSETS_DIR / "crop_req60_scraping_mitigation.png")
    
    # 8. Req 61: Security Analyst Workflows
    im2 = Image.open(ASSETS_DIR / "extra_img_2.png")
    crop_61 = im2.crop((120, 50, 950, 350))
    crop_61.save(ASSETS_DIR / "crop_req61_analyst_workflow.png")
    
    # 9. Req 62: Immutable Audit Logging
    crop_62 = im2.crop((120, 120, 950, 430))
    crop_62.save(ASSETS_DIR / "crop_req62_audit_logging.png")
    
    # 10. Req 64: Compliance Reporting & Telemetry
    im_ov = Image.open(ASSETS_DIR / "ui_overview.png")
    crop_64 = im_ov.crop((100, 40, 950, 360))
    crop_64.save(ASSETS_DIR / "crop_req64_compliance_reporting.png")
    
    # 11. Req 67: Classified Dataset Access Prevention Policy
    crop_67 = im3.crop((120, 100, 950, 400))
    crop_67.save(ASSETS_DIR / "crop_req67_dataset_access.png")
    
    # 12. Req 69: Prompt Injection Defense
    crop_69 = im_p2_1.crop((150, 150, 1800, 700))
    crop_69.save(ASSETS_DIR / "crop_req69_prompt_injection.png")
    
    # 13. Req 71: Automated Remediation Actions
    crop_71 = im2.crop((120, 150, 950, 450))
    crop_71.save(ASSETS_DIR / "crop_req71_automated_remediation.png")
    
    print("All 13 crops successfully created!")

if __name__ == "__main__":
    create_crops()
