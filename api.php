<?php

if (php_sapi_name() == 'cli') {
    $post = getopt('', ['post:'])['post'] ?? '';
    parse_str($post, $_POST);
}

if ($_SERVER['REQUEST_METHOD'] == 'POST') {
    $id = $_POST['id'] ?? 1;
    $name = $_POST['name'] ?? 'user';
    $ua = $_SERVER['HTTP_USER_AGENT'] ?? 'Unknown Agent';

    echo $id . '-' . $name . ' from ' . $ua;
} else {
    echo 'Hello World';
}

