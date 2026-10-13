ALTER TABLE beta_feedback
 ADD COLUMN request_reference uuid,
 ADD COLUMN source_page text NOT NULL DEFAULT '' CHECK(char_length(source_page)<=180);
