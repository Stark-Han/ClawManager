ALTER TABLE instances
  MODIFY COLUMN runtime_type ENUM('desktop', 'shell', 'gateway') NOT NULL DEFAULT 'desktop';
