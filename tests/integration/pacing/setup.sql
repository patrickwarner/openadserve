-- Pacing Strategy Test Data Setup
-- Creates isolated test data for comparing pacing strategies

-- Create test publisher
INSERT INTO publishers (id, name, domain, api_key)
VALUES (999, 'Pacing Test Publisher', 'pacing-test.example.com', 'pacing-test-key')
ON CONFLICT (id) DO NOTHING;

-- Create test placements
INSERT INTO placements (id, publisher_id, width, height, formats) VALUES
('test-header', 999, 728, 90, ARRAY['html']),
('test-sidebar', 999, 160, 600, ARRAY['html']),
('test-content', 999, 300, 250, ARRAY['html'])
ON CONFLICT (id) DO NOTHING;

-- Create test campaigns for each pacing strategy
INSERT INTO campaigns (id, publisher_id, name) VALUES
(999001, 999, 'ASAP Pacing Test Campaign'),
(999002, 999, 'Even Pacing Test Campaign'),
(999003, 999, 'PID Pacing Test Campaign')
ON CONFLICT (id) DO NOTHING;

-- Create line items with different pacing strategies
-- All start now and run for 24 hours with configurable daily impression cap
-- Placeholders will be replaced by run_test.sh:
-- {{DAILY_IMPRESSION_CAP}} - Daily impression limit
-- {{CPM}} - Cost per thousand impressions
-- {{BUDGET_AMOUNT}} - Calculated budget (daily_cap * cpm/1000 * multiplier)
INSERT INTO line_items (
    id, campaign_id, publisher_id, name, start_date, end_date,
    daily_impression_cap, daily_click_cap, pace_type, priority,
    frequency_cap, frequency_window, active, cpm, cpc, ecpm,
    budget_type, budget_amount, spend, li_type
) VALUES
-- ASAP Pacing Line Item
(1000001, 999001, 999, 'ASAP Pacing', NOW() AT TIME ZONE 'America/New_York', NOW() AT TIME ZONE 'America/New_York' + INTERVAL '24 hours',
 {{DAILY_IMPRESSION_CAP}}, 0, 'asap', 'high', 10, 300, true, {{CPM}}, 0.0, {{CPM}},
 'cpm', {{BUDGET_AMOUNT}}, 0.0, 'direct'),

-- Even Pacing Line Item
(1000002, 999002, 999, 'Even Pacing', NOW() AT TIME ZONE 'America/New_York', NOW() AT TIME ZONE 'America/New_York' + INTERVAL '24 hours',
 {{DAILY_IMPRESSION_CAP}}, 0, 'even', 'high', 10, 300, true, {{CPM}}, 0.0, {{CPM}},
 'cpm', {{BUDGET_AMOUNT}}, 0.0, 'direct'),

-- PID Pacing Line Item
(1000003, 999003, 999, 'PID Pacing', NOW() AT TIME ZONE 'America/New_York', NOW() AT TIME ZONE 'America/New_York' + INTERVAL '24 hours',
 {{DAILY_IMPRESSION_CAP}}, 0, 'pid', 'high', 10, 300, true, {{CPM}}, 0.0, {{CPM}},
 'cpm', {{BUDGET_AMOUNT}}, 0.0, 'direct')
ON CONFLICT (id) DO UPDATE SET
    pace_type = EXCLUDED.pace_type,
    daily_impression_cap = EXCLUDED.daily_impression_cap,
    daily_click_cap = EXCLUDED.daily_click_cap,
    start_date = NOW() AT TIME ZONE 'America/New_York',
    end_date = NOW() AT TIME ZONE 'America/New_York' + INTERVAL '24 hours',
    active = EXCLUDED.active;

-- Create creatives for each line item and placement combination
INSERT INTO creatives (id, placement_id, line_item_id, campaign_id, publisher_id, html, width, height, format) VALUES
-- ASAP Pacing Creatives (Red theme)
(10000011, 'test-header', 1000001, 999001, 999,
 '<div style="width:728px;height:90px;background:#ff6b6b;border:2px solid #333;display:flex;align-items:center;justify-content:center;font-family:sans-serif;color:white;font-weight:bold;text-align:center;"><div><div style="font-size:16px;">ASAP PACING</div><div style="font-size:12px;margin-top:5px;">Fast Delivery</div><div style="font-size:10px;margin-top:2px;">Campaign 999001</div></div></div>',
 728, 90, 'html'),

(10000012, 'test-sidebar', 1000001, 999001, 999,
 '<div style="width:160px;height:600px;background:#ff6b6b;border:2px solid #333;display:flex;align-items:center;justify-content:center;font-family:sans-serif;color:white;font-weight:bold;text-align:center;"><div><div style="font-size:14px;">ASAP</div><div style="font-size:14px;">PACING</div><div style="font-size:10px;margin-top:5px;">Fast Delivery</div><div style="font-size:8px;margin-top:2px;">999001</div></div></div>',
 160, 600, 'html'),

(10000013, 'test-content', 1000001, 999001, 999,
 '<div style="width:300px;height:250px;background:#ff6b6b;border:2px solid #333;display:flex;align-items:center;justify-content:center;font-family:sans-serif;color:white;font-weight:bold;text-align:center;"><div><div style="font-size:16px;">ASAP PACING</div><div style="font-size:12px;margin-top:5px;">Fast Delivery</div><div style="font-size:10px;margin-top:2px;">Campaign 999001</div></div></div>',
 300, 250, 'html'),

-- Even Pacing Creatives (Green theme)
(10000021, 'test-header', 1000002, 999002, 999,
 '<div style="width:728px;height:90px;background:#4ecdc4;border:2px solid #333;display:flex;align-items:center;justify-content:center;font-family:sans-serif;color:white;font-weight:bold;text-align:center;"><div><div style="font-size:16px;">EVEN PACING</div><div style="font-size:12px;margin-top:5px;">Steady Delivery</div><div style="font-size:10px;margin-top:2px;">Campaign 999002</div></div></div>',
 728, 90, 'html'),

(10000022, 'test-sidebar', 1000002, 999002, 999,
 '<div style="width:160px;height:600px;background:#4ecdc4;border:2px solid #333;display:flex;align-items:center;justify-content:center;font-family:sans-serif;color:white;font-weight:bold;text-align:center;"><div><div style="font-size:14px;">EVEN</div><div style="font-size:14px;">PACING</div><div style="font-size:10px;margin-top:5px;">Steady Delivery</div><div style="font-size:8px;margin-top:2px;">999002</div></div></div>',
 160, 600, 'html'),

(10000023, 'test-content', 1000002, 999002, 999,
 '<div style="width:300px;height:250px;background:#4ecdc4;border:2px solid #333;display:flex;align-items:center;justify-content:center;font-family:sans-serif;color:white;font-weight:bold;text-align:center;"><div><div style="font-size:16px;">EVEN PACING</div><div style="font-size:12px;margin-top:5px;">Steady Delivery</div><div style="font-size:10px;margin-top:2px;">Campaign 999002</div></div></div>',
 300, 250, 'html'),

-- PID Pacing Creatives (Blue theme)
(10000031, 'test-header', 1000003, 999003, 999,
 '<div style="width:728px;height:90px;background:#45b7d1;border:2px solid #333;display:flex;align-items:center;justify-content:center;font-family:sans-serif;color:white;font-weight:bold;text-align:center;"><div><div style="font-size:16px;">PID PACING</div><div style="font-size:12px;margin-top:5px;">Smart Delivery</div><div style="font-size:10px;margin-top:2px;">Campaign 999003</div></div></div>',
 728, 90, 'html'),

(10000032, 'test-sidebar', 1000003, 999003, 999,
 '<div style="width:160px;height:600px;background:#45b7d1;border:2px solid #333;display:flex;align-items:center;justify-content:center;font-family:sans-serif;color:white;font-weight:bold;text-align:center;"><div><div style="font-size:14px;">PID</div><div style="font-size:14px;">PACING</div><div style="font-size:10px;margin-top:5px;">Smart Delivery</div><div style="font-size:8px;margin-top:2px;">999003</div></div></div>',
 160, 600, 'html'),

(10000033, 'test-content', 1000003, 999003, 999,
 '<div style="width:300px;height:250px;background:#45b7d1;border:2px solid #333;display:flex;align-items:center;justify-content:center;font-family:sans-serif;color:white;font-weight:bold;text-align:center;"><div><div style="font-size:16px;">PID PACING</div><div style="font-size:12px;margin-top:5px;">Smart Delivery</div><div style="font-size:10px;margin-top:2px;">Campaign 999003</div></div></div>',
 300, 250, 'html')
ON CONFLICT (id) DO NOTHING;

-- Display setup summary
SELECT 'Test data setup completed!' as status;
SELECT
    'Publisher: ' || name || ' (ID: ' || id || ', API Key: ' || api_key || ')' as info
FROM publishers WHERE id = 999;

SELECT
    'Campaign: ' || name || ' (ID: ' || id || ')' as campaigns
FROM campaigns WHERE publisher_id = 999 ORDER BY id;

SELECT
    'Line Item: ' || id || ' (' || pace_type || ' pacing, ' ||
    daily_impression_cap || ' daily cap)' as line_items
FROM line_items WHERE publisher_id = 999 ORDER BY id;
