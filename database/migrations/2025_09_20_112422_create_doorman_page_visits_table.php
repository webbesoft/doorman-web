<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration
{
    public function up()
    {
        Schema::create('doorman_page_visits', function (Blueprint $table) {
            $table->id();
            $table->string('page');
            $table->string('identifier');
            $table->string('identifier_type');
            $table->date('date');
            $table->timestamps();

            $table->unique(['page', 'identifier', 'identifier_type', 'date'], 'unique_page_identifier_date');
        });
    }

    public function down()
    {
        Schema::dropIfExists('doorman_page_visits');
    }
};
